package internal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// tgz builds an npm-style tarball (everything under "package/").
func tgz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		mode := int64(0o644)
		if strings.HasPrefix(body, "#!") {
			mode = 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Name: "package/" + name, Mode: mode, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func sri(b []byte) string {
	s := sha512.Sum512(b)
	return "sha512-" + base64.StdEncoding.EncodeToString(s[:])
}

type fakeVersion struct {
	deps, optional, peers map[string]string
	bin                   string
	os                    []string
}

// fakeRegistry serves packuments and tarballs and counts requests of each kind.
type fakeRegistry struct {
	*httptest.Server
	meta, tarballs atomic.Int64
	tamper         bool
}

func newFakeRegistry(t *testing.T, pkgs map[string]map[string]fakeVersion, latest map[string]string) *fakeRegistry {
	t.Helper()
	f := &fakeRegistry{}
	blobs := map[string][]byte{}
	docs := map[string]*Packument{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasPrefix(p, "-/") {
			f.tarballs.Add(1)
			b, ok := blobs[p]
			if !ok {
				http.NotFound(w, r)
				return
			}
			if f.tamper {
				b = append(append([]byte{}, b...), 0)
			}
			w.Write(b)
			return
		}
		f.meta.Add(1)
		doc, ok := docs[p]
		if !ok {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(f.Close)
	for name, versions := range pkgs {
		doc := &Packument{Name: name, DistTags: map[string]string{"latest": latest[name]}, Versions: map[string]Manifest{}}
		for ver, fv := range versions {
			files := map[string]string{"package.json": `{"name":"` + name + `","version":"` + ver + `"}`,
				"index.js": "module.exports = " + `"` + name + "@" + ver + `"`}
			m := Manifest{Name: name, Version: ver, Dependencies: fv.deps, OptionalDependencies: fv.optional,
				PeerDependencies: fv.peers, OS: fv.os}
			if fv.bin != "" {
				files[fv.bin] = "#!/usr/bin/env node\n"
				m.Bin, _ = json.Marshal(fv.bin)
			}
			key := "-/" + strings.ReplaceAll(name, "/", "+") + "-" + ver + ".tgz"
			blobs[key] = tgz(t, files)
			m.Dist = &Dist{Tarball: f.URL + "/" + key, Integrity: sri(blobs[key])}
			doc.Versions[ver] = m
		}
		docs[name] = doc
	}
	return f
}

func defaultRegistry(t *testing.T) *fakeRegistry {
	return newFakeRegistry(t, map[string]map[string]fakeVersion{
		"a": {"1.0.0": {deps: map[string]string{"b": "^1"}, bin: "cli.js"}},
		"b": {"1.0.0": {}, "1.1.0": {}, "2.0.0": {}},
		"c": {"1.0.0": {deps: map[string]string{"b": "^2"}}},
	}, map[string]string{"a": "1.0.0", "b": "2.0.0", "c": "1.0.0"})
}

func writeTestFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, p string, v any) {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func install(t *testing.T, reg *fakeRegistry, dir, store string, args ...string) *Result {
	t.Helper()
	res, err := Install(Options{Dir: dir, Store: store, Registry: reg.URL, Args: args, Concurrency: 8})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func realpath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestInstallLayoutLockfileAndCache(t *testing.T) {
	reg := defaultRegistry(t)
	dir, store := t.TempDir(), t.TempDir()
	store = realpath(t, store)
	writeTestFile(t, filepath.Join(dir, "package.json"), `{"name":"proj","dependencies":{"a":"^1","c":"1.0.0"}}`)

	res := install(t, reg, dir, store)
	if res.Packages != 4 || res.Downloaded != 4 {
		t.Fatalf("got %+v, want 4 packages downloaded", res)
	}

	// Each package sees exactly the version it resolved, via symlinks into the store.
	cases := map[string]string{
		"node_modules/a": "a@1.0.0/node_modules/a",
		"node_modules/c": "c@1.0.0/node_modules/c",
	}
	for link, want := range cases {
		if got := realpath(t, filepath.Join(dir, link)); got != filepath.Join(store, want) {
			t.Errorf("%s -> %s, want %s", link, got, want)
		}
	}
	if got := realpath(t, filepath.Join(store, "a@1.0.0/node_modules/b")); !strings.HasSuffix(got, "b@1.1.0/node_modules/b") {
		t.Errorf("a's b -> %s, want 1.1.0", got)
	}
	if got := realpath(t, filepath.Join(store, "c@1.0.0/node_modules/b")); !strings.HasSuffix(got, "b@2.0.0/node_modules/b") {
		t.Errorf("c's b -> %s, want 2.0.0", got)
	}
	if _, err := os.Stat(filepath.Join(store, "a@1.0.0/node_modules/a/package.json")); err != nil {
		t.Errorf("tarball not extracted with package/ prefix stripped: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "node_modules/.bin/a")); err != nil || fi.Mode()&0o111 == 0 {
		t.Errorf("bin not linked/executable: %v", err)
	}

	// Lockfile is hoisted like npm: first b wins the top level, the conflicting one nests.
	var lock Lockfile
	readJSON(t, filepath.Join(dir, "package-lock.json"), &lock)
	if lock.LockfileVersion != 3 {
		t.Errorf("lockfileVersion %d", lock.LockfileVersion)
	}
	want := map[string]string{"node_modules/a": "1.0.0", "node_modules/b": "1.1.0", "node_modules/c": "1.0.0", "node_modules/c/node_modules/b": "2.0.0"}
	for p, v := range want {
		if e := lock.Packages[p]; e == nil || e.Version != v || e.Integrity == "" || e.Resolved == "" {
			t.Errorf("lock %s = %+v, want version %s", p, e, v)
		}
	}
	if len(lock.Packages) != len(want)+1 {
		t.Errorf("lock has %d entries, want %d", len(lock.Packages), len(want)+1)
	}

	// Warm store + lockfile: no network at all.
	reg.meta.Store(0)
	reg.tarballs.Store(0)
	os.RemoveAll(filepath.Join(dir, "node_modules"))
	if res := install(t, reg, dir, store); res.Downloaded != 0 || reg.meta.Load()+reg.tarballs.Load() != 0 {
		t.Errorf("warm install made %d metadata + %d tarball requests", reg.meta.Load(), reg.tarballs.Load())
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules/a/index.js")); err != nil {
		t.Error(err)
	}

	// Cold store + lockfile: tarballs only, no metadata requests.
	install(t, reg, dir, t.TempDir())
	if reg.meta.Load() != 0 || reg.tarballs.Load() != 4 {
		t.Errorf("lockfile install made %d metadata + %d tarball requests, want 0 + 4", reg.meta.Load(), reg.tarballs.Load())
	}
}

func TestInstallAddSavesToPackageJSON(t *testing.T) {
	reg := defaultRegistry(t)
	dir, store := t.TempDir(), t.TempDir()
	writeTestFile(t, filepath.Join(dir, "package.json"), `{"name":"proj","scripts":{"x":"y"}}`)

	install(t, reg, dir, store, "b", "a@^1")
	var pj struct {
		Dependencies    map[string]string
		DevDependencies map[string]string
	}
	readJSON(t, filepath.Join(dir, "package.json"), &pj)
	if pj.Dependencies["b"] != "^2.0.0" || pj.Dependencies["a"] != "^1.0.0" {
		t.Errorf("dependencies = %v", pj.Dependencies)
	}

	if _, err := Install(Options{Dir: dir, Store: store, Registry: reg.URL, Args: []string{"b@~1.0.0"}, SaveDev: true, Concurrency: 8}); err != nil {
		t.Fatal(err)
	}
	pj.Dependencies, pj.DevDependencies = nil, nil
	readJSON(t, filepath.Join(dir, "package.json"), &pj)
	if _, still := pj.Dependencies["b"]; still || pj.DevDependencies["b"] != "~1.0.0" {
		t.Errorf("b not moved to devDependencies: %v", pj)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	if !strings.HasPrefix(string(b), "{\n  \"name\": \"proj\",\n  \"scripts\"") {
		t.Errorf("key order not preserved:\n%s", b)
	}
	if got := realpath(t, filepath.Join(dir, "node_modules/b")); !strings.HasSuffix(got, "b@1.0.0/node_modules/b") {
		t.Errorf("node_modules/b -> %s", got)
	}
}

func TestInstallRejectsTamperedTarball(t *testing.T) {
	reg := defaultRegistry(t)
	reg.tamper = true
	dir, store := t.TempDir(), t.TempDir()
	writeTestFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"b":"1.0.0"}}`)
	_, err := Install(Options{Dir: dir, Store: store, Registry: reg.URL, Concurrency: 8})
	if err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("want integrity error, got %v", err)
	}
	if entries, _ := os.ReadDir(store); len(entries) != 0 {
		t.Errorf("store polluted after failed verify: %v", entries)
	}
}

func TestInstallPlatformOptionalAndPeers(t *testing.T) {
	reg := newFakeRegistry(t, map[string]map[string]fakeVersion{
		"tool":   {"1.0.0": {optional: map[string]string{"native": "1.0.0", "missing": "1.0.0"}}},
		"native": {"1.0.0": {os: []string{"plan9-nope"}}},
		"plugin": {"1.0.0": {peers: map[string]string{"host": ">=1"}}},
		"host":   {"1.0.0": {}, "2.0.0": {}},
	}, map[string]string{"tool": "1.0.0", "native": "1.0.0", "plugin": "1.0.0", "host": "2.0.0"})
	dir, store := t.TempDir(), t.TempDir()
	writeTestFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"tool":"1","plugin":"1","host":"1.0.0"}}`)

	res := install(t, reg, dir, store)
	if res.Packages != 3 {
		t.Errorf("installed %d packages, want 3 (native skipped, missing ignored)", res.Packages)
	}
	if _, err := os.Stat(filepath.Join(store, "native@1.0.0")); err == nil {
		t.Error("wrong-platform optional dep was downloaded")
	}
	var lock Lockfile
	readJSON(t, filepath.Join(dir, "package-lock.json"), &lock)
	if lock.Packages["node_modules/native"] == nil {
		t.Error("wrong-platform optional dep should still be in the lockfile")
	}
	// The peer links to the project's host@1.0.0, not the registry's latest 2.0.0.
	if got := realpath(t, filepath.Join(store, "plugin@1.0.0/node_modules/host")); !strings.HasSuffix(got, "host@1.0.0/node_modules/host") {
		t.Errorf("peer host -> %s, want project's 1.0.0", got)
	}
}

func TestInstallPrunesExtraneous(t *testing.T) {
	reg := defaultRegistry(t)
	dir, store := t.TempDir(), t.TempDir()
	writeTestFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"a":"^1","c":"1.0.0"}}`)
	install(t, reg, dir, store)

	nmDir := filepath.Join(dir, "node_modules")
	for _, d := range []string{".cache/tool", "stray/lib", "@s/old"} {
		os.MkdirAll(filepath.Join(nmDir, d), 0o755)
	}
	writeTestFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"c":"1.0.0"}}`)
	res := install(t, reg, dir, store)

	if res.Removed != 3 {
		t.Errorf("removed %d, want 3 (a, stray, @s/old)", res.Removed)
	}
	for _, gone := range []string{"a", "stray", "@s", ".bin/a"} {
		if _, err := os.Lstat(filepath.Join(nmDir, gone)); err == nil {
			t.Errorf("%s should have been pruned", gone)
		}
	}
	for _, kept := range []string{"c", ".cache/tool"} {
		if _, err := os.Lstat(filepath.Join(nmDir, kept)); err != nil {
			t.Errorf("%s should be kept: %v", kept, err)
		}
	}
	var lock Lockfile
	readJSON(t, filepath.Join(dir, "package-lock.json"), &lock)
	if lock.Packages["node_modules/a"] != nil {
		t.Error("removed dependency still in lockfile")
	}
}

func TestInstallDedupesToProjectVersion(t *testing.T) {
	reg := defaultRegistry(t)
	dir, store := t.TempDir(), t.TempDir()
	// a wants b@^1 (highest 1.1.0), but the project pins b@1.0.0 which also fits: npm keeps one copy.
	writeTestFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"a":"^1","b":"1.0.0"}}`)
	res := install(t, reg, dir, store)

	if got := realpath(t, filepath.Join(store, "a@1.0.0/node_modules/b")); !strings.HasSuffix(got, "b@1.0.0/node_modules/b") {
		t.Errorf("a's b -> %s, want project's 1.0.0", got)
	}
	if res.Packages != 2 {
		t.Errorf("packages = %d, want 2", res.Packages)
	}
	var lock Lockfile
	readJSON(t, filepath.Join(dir, "package-lock.json"), &lock)
	if len(lock.Packages) != 3 || lock.Packages["node_modules/b"].Version != "1.0.0" {
		t.Errorf("lockfile not deduped: %v", sortedKeys(lock.Packages))
	}
}
