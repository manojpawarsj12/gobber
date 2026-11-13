package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
)

// Lockfile is npm's package-lock.json, lockfileVersion 3.
type Lockfile struct {
	Name            string               `json:"name,omitempty"`
	Version         string               `json:"version,omitempty"`
	LockfileVersion int                  `json:"lockfileVersion"`
	Requires        bool                 `json:"requires"`
	Packages        map[string]*Manifest `json:"packages"`
}

// ReadLockfile returns nil, nil when there is no lockfile.
func ReadLockfile(file string) (*Lockfile, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var l Lockfile
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("%s: %w (delete it to re-resolve)", file, err)
	}
	return &l, nil
}

func (l *Lockfile) Write(file string) error {
	b, err := marshal(l, "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(b, '\n'), 0o644)
}

const nm = "node_modules/"

// lockedManifests indexes registry packages in the lockfile by real package name.
func lockedManifests(l *Lockfile) map[string][]Manifest {
	out := map[string][]Manifest{}
	if l == nil {
		return out
	}
	seen := map[string]bool{}
	for p, m := range l.Packages {
		i := strings.LastIndex(p, nm)
		if m == nil || i < 0 || m.Version == "" || m.Resolved == "" {
			continue // root, workspace or linked entries
		}
		mm := *m
		if mm.Name == "" {
			mm.Name = p[i+len(nm):]
		}
		mm.Dev = false
		if key := mm.Name + "@" + mm.Version; !seen[key] {
			seen[key] = true
			out[mm.Name] = append(out[mm.Name], mm)
		}
	}
	return out
}

// buildLockfile lays the graph out the way npm would hoist it, so `npm ci` can use the result.
func buildLockfile(pj *PackageJSON, roots map[string]*Node) *Lockfile {
	l := &Lockfile{Name: pj.Name, Version: pj.Version, LockfileVersion: 3, Requires: true,
		Packages: map[string]*Manifest{"": {Name: pj.Name, Version: pj.Version,
			Dependencies: pj.Dependencies, DevDependencies: pj.DevDependencies}}}

	prod := map[*Node]bool{}
	var mark func(*Node)
	mark = func(n *Node) {
		if !prod[n] {
			prod[n] = true
			for _, c := range n.Children {
				mark(c)
			}
		}
	}
	for alias := range pj.Dependencies {
		if n := roots[alias]; n != nil {
			mark(n)
		}
	}

	type item struct {
		path string
		n    *Node
	}
	placed := map[string]*Node{}
	var queue []item
	place := func(p, alias string, n *Node) {
		m := n.M
		m.Name = ""
		if alias != n.M.Name {
			m.Name = n.M.Name // npm records the real name for aliased installs
		}
		m.Dev = !prod[n]
		l.Packages[p] = &m
		placed[p] = n
		queue = append(queue, item{p, n})
	}
	for _, alias := range sortedKeys(roots) {
		place(nm+alias, alias, roots[alias])
	}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		for _, alias := range sortedKeys(it.n.Children) {
			c := it.n.Children[alias]
			found := ""
			for dir := it.path; ; dir = parentPath(dir) {
				cand := nm + alias
				if dir != "" {
					cand = dir + "/" + cand
				}
				if placed[cand] != nil {
					found = cand
					break
				}
				if dir == "" {
					break
				}
			}
			switch {
			case found == "":
				place(nm+alias, alias, c)
			case placed[found] != c:
				place(it.path+"/"+nm+alias, alias, c)
			}
		}
	}
	return l
}

// parentPath("node_modules/a/node_modules/b") == "node_modules/a"; top level returns "".
func parentPath(p string) string {
	if i := strings.LastIndex(p, "/"+nm); i >= 0 {
		return p[:i]
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
