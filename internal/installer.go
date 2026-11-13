package internal

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Masterminds/semver/v3"
)

type Options struct {
	Dir         string   // project directory (holds package.json)
	Store       string   // global content store shared by all projects
	Registry    string   // registry base URL
	Args        []string // packages to add, e.g. "react@^18"
	SaveDev     bool     // save Args to devDependencies
	Production  bool     // skip devDependencies and leave the lockfile alone
	Concurrency int      // max parallel HTTP requests
}

type Result struct {
	Packages   int
	Downloaded int64
}

// Node is one resolved name@version; Children are keyed by the name it is required as.
type Node struct {
	M        Manifest
	Children map[string]*Node
	Skip     bool // os/cpu don't match this machine
}

type peerReq struct {
	parent      *Node
	alias, spec string
}

type installer struct {
	o          Options
	reg        *Registry
	locked     map[string][]Manifest
	mu         sync.Mutex
	nodes      map[string]*Node
	packs      map[string]*packCall
	peers      []peerReq
	wg         sync.WaitGroup
	errs       []error
	downloaded atomic.Int64
}

type packCall struct {
	done chan struct{}
	p    *Packument
	err  error
}

// Install resolves package.json (preferring versions already in package-lock.json),
// fills the global store, symlinks node_modules into it and writes both manifests.
func Install(o Options) (*Result, error) {
	pjPath, lockPath := filepath.Join(o.Dir, "package.json"), filepath.Join(o.Dir, "package-lock.json")
	pj, err := ReadPackageJSON(pjPath)
	if err != nil {
		return nil, err
	}
	lock, err := ReadLockfile(lockPath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(pjPath); err != nil && len(o.Args) == 0 {
		return nil, fmt.Errorf("no package.json in %s and no packages given", o.Dir)
	}
	if err := os.MkdirAll(o.Store, 0o755); err != nil {
		return nil, err
	}
	in := &installer{o: o, reg: NewRegistry(o.Registry, o.Concurrency), locked: lockedManifests(lock),
		nodes: map[string]*Node{}, packs: map[string]*packCall{}}

	// 1. Resolve packages named on the command line so their saved range is known.
	added := make([]*Node, len(o.Args))
	for i, arg := range o.Args {
		i, arg := i, arg
		in.spawn(func() error {
			alias, spec := ParseArg(arg)
			n, err := in.resolve(alias, spec)
			added[i] = n
			return err
		})
	}
	if err := in.wait(); err != nil {
		return nil, err
	}
	for i, arg := range o.Args {
		alias, spec := ParseArg(arg)
		to, from := &pj.Dependencies, &pj.DevDependencies
		if o.SaveDev {
			to, from = from, to
		}
		if *to == nil {
			*to = map[string]string{}
		}
		(*to)[alias] = saveSpec(spec, added[i].M.Version)
		delete(*from, alias)
	}

	// 2. Resolve every root dependency (mostly cache hits after step 1).
	specs := map[string]string{}
	if !o.Production {
		for k, v := range pj.DevDependencies {
			specs[k] = v
		}
	}
	for k, v := range pj.Dependencies {
		specs[k] = v
	}
	roots := map[string]*Node{}
	for alias, spec := range specs {
		alias, spec := alias, spec
		in.spawn(func() error {
			n, err := in.resolve(alias, spec)
			if err != nil {
				return err
			}
			in.mu.Lock()
			roots[alias] = n
			in.mu.Unlock()
			return nil
		})
	}
	if err := in.wait(); err != nil {
		return nil, err
	}
	if err := in.resolvePeers(roots); err != nil {
		return nil, err
	}

	// 3. Link. Store entries get their deps as sibling symlinks, so Node's
	// resolution from the real (store) path finds exactly the resolved versions.
	installed := 0
	for _, n := range in.nodes {
		if n.Skip {
			continue
		}
		installed++
		for alias, c := range n.Children {
			if alias == n.M.Name || c.Skip {
				continue // self-dependency is already there; skipped platforms aren't downloaded
			}
			if err := symlink(in.modulePath(c), filepath.Join(in.entryDir(n), "node_modules", alias)); err != nil {
				return nil, err
			}
		}
	}
	nmDir := filepath.Join(o.Dir, "node_modules")
	for alias, n := range roots {
		if n.Skip {
			return nil, fmt.Errorf("%s@%s is not available for this platform", n.M.Name, n.M.Version)
		}
		if err := symlink(in.modulePath(n), filepath.Join(nmDir, alias)); err != nil {
			return nil, err
		}
		if err := linkBins(nmDir, alias, n, in.modulePath(n)); err != nil {
			return nil, err
		}
	}

	if !o.Production {
		if err := buildLockfile(pj, roots).Write(lockPath); err != nil {
			return nil, err
		}
	}
	if len(o.Args) > 0 {
		if err := pj.Write(pjPath); err != nil {
			return nil, err
		}
	}
	return &Result{Packages: installed, Downloaded: in.downloaded.Load()}, nil
}

func (in *installer) spawn(f func() error) {
	in.wg.Add(1)
	go func() {
		defer in.wg.Done()
		if err := f(); err != nil {
			in.mu.Lock()
			in.errs = append(in.errs, err)
			in.mu.Unlock()
		}
	}()
}

func (in *installer) wait() error {
	in.wg.Wait()
	return errors.Join(in.errs...)
}

// resolve picks a version (lockfile first, registry second) and returns its node.
func (in *installer) resolve(alias, spec string) (*Node, error) {
	name, rng := alias, spec
	if strings.HasPrefix(spec, "npm:") {
		name, rng = ParseArg(spec[len("npm:"):])
	}
	if !validName(alias) || !validName(name) {
		return nil, fmt.Errorf("invalid package name %q", alias)
	}
	m, ok := pickLocked(in.locked[name], rng)
	if !ok {
		p, err := in.packument(name)
		if err != nil {
			return nil, err
		}
		v, err := pickVersion(p, rng)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		m = fromRegistry(p.Versions[v])
		m.Name, m.Version = name, v
	}
	return in.node(m)
}

// packument fetches each package's metadata at most once per install.
func (in *installer) packument(name string) (*Packument, error) {
	in.mu.Lock()
	c := in.packs[name]
	if c == nil {
		c = &packCall{done: make(chan struct{})}
		in.packs[name] = c
		in.mu.Unlock()
		c.p, c.err = in.reg.Packument(name)
		close(c.done)
	} else {
		in.mu.Unlock()
		<-c.done
	}
	return c.p, c.err
}

// node dedupes name@version; a new node starts downloading immediately and resolves its deps in parallel.
func (in *installer) node(m Manifest) (*Node, error) {
	if _, err := semver.NewVersion(m.Version); err != nil {
		return nil, fmt.Errorf("%s: invalid version %q", m.Name, m.Version)
	}
	key := m.Name + "@" + m.Version
	in.mu.Lock()
	if n, ok := in.nodes[key]; ok {
		in.mu.Unlock()
		return n, nil
	}
	n := &Node{M: m, Children: map[string]*Node{}, Skip: !platformOK(&m)}
	in.nodes[key] = n
	if n.Skip {
		in.mu.Unlock()
		return n, nil // kept for the lockfile so other platforms can use it, never downloaded here
	}
	for alias, spec := range m.PeerDependencies {
		_, isDep := m.Dependencies[alias]
		if !isDep && !m.PeerDependenciesMeta[alias].Optional {
			in.peers = append(in.peers, peerReq{n, alias, spec})
		}
	}
	in.mu.Unlock()

	in.spawn(func() error { return in.fetch(n) })
	for alias, spec := range m.Dependencies {
		if _, opt := m.OptionalDependencies[alias]; !opt {
			in.child(n, alias, spec, false)
		}
	}
	for alias, spec := range m.OptionalDependencies {
		in.child(n, alias, spec, true)
	}
	return n, nil
}

func (in *installer) child(parent *Node, alias, spec string, optional bool) {
	in.spawn(func() error {
		c, err := in.resolve(alias, spec)
		if err == nil && c.Skip && !optional {
			err = fmt.Errorf("%s@%s is not available for this platform", c.M.Name, c.M.Version)
		}
		if err != nil {
			if optional {
				return nil
			}
			return fmt.Errorf("%s@%s requires %s: %w", parent.M.Name, parent.M.Version, alias, err)
		}
		in.mu.Lock()
		parent.Children[alias] = c
		in.mu.Unlock()
		return nil
	})
}

// resolvePeers links peer deps to the project's own copy when it satisfies the
// range (so e.g. there is one React), otherwise resolves them like normal deps.
// shortcut: one store entry per name@version, so peers are not per-project like pnpm; fine unless two projects need different peers for the same package version.
func (in *installer) resolvePeers(roots map[string]*Node) error {
	for {
		in.mu.Lock()
		peers := in.peers
		in.peers = nil
		in.mu.Unlock()
		if len(peers) == 0 {
			return nil
		}
		for _, p := range peers {
			p := p
			if r := roots[p.alias]; r != nil {
				if c, err := semver.NewConstraint(p.spec); err == nil && satisfies(c, r.M.Version) {
					in.mu.Lock()
					p.parent.Children[p.alias] = r
					in.mu.Unlock()
					continue
				}
			}
			in.child(p.parent, p.alias, p.spec, true) // npm only warns on unmet peers
		}
		if err := in.wait(); err != nil {
			return err
		}
	}
}

func (in *installer) entryDir(n *Node) string {
	return filepath.Join(in.o.Store, strings.ReplaceAll(n.M.Name, "/", "+")+"@"+n.M.Version)
}

func (in *installer) modulePath(n *Node) string {
	return filepath.Join(in.entryDir(n), "node_modules", n.M.Name)
}

// fetch downloads and verifies a tarball into the store unless it is already there.
// Extraction goes to a temp dir that is renamed into place, so a store entry is never half-written.
func (in *installer) fetch(n *Node) error {
	dir := in.entryDir(n)
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	if n.M.Resolved == "" {
		return fmt.Errorf("%s@%s: no tarball URL", n.M.Name, n.M.Version)
	}
	body, err := in.reg.get(n.M.Resolved, "")
	if err != nil {
		return err
	}
	defer body.Close()
	tmp, err := os.MkdirTemp(in.o.Store, ".tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := extractVerified(body, n.M.Integrity, filepath.Join(tmp, "node_modules", n.M.Name)); err != nil {
		return fmt.Errorf("%s@%s: %w", n.M.Name, n.M.Version, err)
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		if _, statErr := os.Stat(dir); statErr == nil {
			return nil // another gobber process got there first
		}
		return err
	}
	in.downloaded.Add(1)
	return nil
}

func symlink(target, link string) error {
	if cur, err := os.Readlink(link); err == nil && cur == target {
		return nil
	}
	if err := os.RemoveAll(link); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	return os.Symlink(target, link)
}

// linkBins creates node_modules/.bin entries for a direct dependency.
func linkBins(nmDir, alias string, n *Node, modPath string) error {
	for bin, rel := range binMap(&n.M) {
		rel = path.Clean(rel)
		if bin == "" || strings.ContainsAny(bin, `/\`) || bin == "." || bin == ".." || !filepath.IsLocal(rel) {
			continue
		}
		_ = os.Chmod(filepath.Join(modPath, rel), 0o755)
		if err := symlink(filepath.Join("..", alias, rel), filepath.Join(nmDir, ".bin", bin)); err != nil {
			return err
		}
	}
	return nil
}
