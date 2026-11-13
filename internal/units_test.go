package internal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArg(t *testing.T) {
	cases := map[string][2]string{
		"react":                  {"react", ""},
		"react@^18":              {"react", "^18"},
		"@types/node":            {"@types/node", ""},
		"@types/node@20":         {"@types/node", "20"},
		"sw@npm:string-width@^4": {"sw", "npm:string-width@^4"},
	}
	for in, want := range cases {
		if n, s := ParseArg(in); n != want[0] || s != want[1] {
			t.Errorf("ParseArg(%q) = %q, %q", in, n, s)
		}
	}
}

func TestValidName(t *testing.T) {
	for _, ok := range []string{"react", "@types/node", "lodash.merge", "Base64"} {
		if !validName(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "..", "../x", "a/b", "@s/../x", ".hidden", "@s/"} {
		if validName(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestPickVersion(t *testing.T) {
	p := &Packument{Name: "x", DistTags: map[string]string{"latest": "1.2.0", "next": "2.0.0-beta.1"},
		Versions: map[string]Manifest{"1.0.0": {}, "1.2.0": {}, "1.3.0": {}, "2.0.0-beta.1": {}, "0.9.0": {}}}
	cases := map[string]string{
		"":       "1.2.0", // latest tag
		"next":   "2.0.0-beta.1",
		"^1":     "1.2.0", // latest wins when it satisfies, like npm
		"^1.2.1": "1.3.0",
		"~1.0":   "1.0.0",
		"<1":     "0.9.0",
		"*":      "1.2.0",
		"1.3.0":  "1.3.0",
	}
	for spec, want := range cases {
		if got, err := pickVersion(p, spec); err != nil || got != want {
			t.Errorf("pickVersion(%q) = %q, %v; want %q", spec, got, err, want)
		}
	}
	for _, bad := range []string{"^3", "github:a/b", "file:../x"} {
		if _, err := pickVersion(p, bad); err == nil {
			t.Errorf("pickVersion(%q) should fail", bad)
		}
	}
}

func TestPickVersionAvoidsDeprecated(t *testing.T) {
	dep := []byte(`"use 1.1.0"`)
	p := &Packument{Name: "x", DistTags: map[string]string{"latest": "2.0.0"},
		Versions: map[string]Manifest{"1.1.0": {}, "1.2.0": {Deprecated: dep}, "2.0.0": {}, "3.0.0": {Deprecated: []byte("false")}}}
	if v, _ := pickVersion(p, "^1"); v != "1.1.0" {
		t.Errorf("^1 picked %s, want non-deprecated 1.1.0", v)
	}
	p.Versions["1.1.0"] = Manifest{Deprecated: dep}
	if v, _ := pickVersion(p, "^1"); v != "1.2.0" {
		t.Errorf("all deprecated: picked %s, want highest 1.2.0", v)
	}
	if v, _ := pickVersion(p, ">=3"); v != "3.0.0" {
		t.Errorf(`"deprecated": false must not count as deprecated, got %s`, v)
	}
}

func TestPickLockedIgnoresTags(t *testing.T) {
	c := []Manifest{{Version: "1.0.0"}, {Version: "1.5.0"}, {Version: "2.0.0"}}
	if _, ok := pickLocked(c, ""); ok {
		t.Error(`"" means latest and must go to the registry`)
	}
	if m, ok := pickLocked(c, "^1"); !ok || m.Version != "1.5.0" {
		t.Errorf("got %v %v", m.Version, ok)
	}
	if _, ok := pickLocked(c, "latest"); ok {
		t.Error("dist-tag must go to the registry")
	}
}

func TestAllowed(t *testing.T) {
	cases := []struct {
		list []string
		v    string
		ok   bool
	}{
		{nil, "linux", true},
		{[]string{"linux"}, "linux", true},
		{[]string{"darwin"}, "linux", false},
		{[]string{"!win32"}, "linux", true},
		{[]string{"!linux"}, "linux", false},
	}
	for _, c := range cases {
		if allowed(c.list, c.v) != c.ok {
			t.Errorf("allowed(%v, %s) != %v", c.list, c.v, c.ok)
		}
	}
}

func TestSaveSpec(t *testing.T) {
	// Same outcomes as `npm install x@<spec>` writes to package.json.
	cases := [][3]string{
		{"", "1.2.3", "^1.2.3"},
		{"latest", "1.2.3", "^1.2.3"},
		{"*", "1.2.3", "^1.2.3"},
		{"1.2.3", "1.2.3", "^1.2.3"},
		{"4", "4.17.21", "^4.17.21"},
		{"^18", "18.3.1", "^18.3.1"},
		{">=1", "2.0.0", "^2.0.0"},
		{"~1.2", "1.2.3", "~1.2"},
		{"1.x <1.2.5", "1.2.4", "1.x <1.2.5"},
		{"^0.2", "0.2.5", "^0.2.5"},
		{"npm:x@1", "1.0.0", "npm:x@1"},
	}
	for _, c := range cases {
		if got := saveSpec(c[0], c[1]); got != c[2] {
			t.Errorf("saveSpec(%q) = %q, want %q", c[0], got, c[2])
		}
	}
}

func rawTgz(t *testing.T, name, body string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
	tw.Write([]byte(body))
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestExtractStripsPrefixAndKeepsExecBit(t *testing.T) {
	dest := t.TempDir()
	// Some tarballs use a top-level dir other than "package/".
	if err := ExtractTarGz(bytes.NewReader(tgz(t, map[string]string{"lib/a.js": "x", "bin/run": "#!/bin/sh"})), dest); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "lib/a.js")); err != nil || string(b) != "x" {
		t.Errorf("lib/a.js: %q %v", b, err)
	}
	if fi, _ := os.Stat(filepath.Join(dest, "bin/run")); fi == nil || fi.Mode()&0o111 == 0 {
		t.Error("exec bit lost")
	}
	if err := ExtractTarGz(bytes.NewReader(rawTgz(t, "node/x.js", "y")), t.TempDir()); err != nil {
		t.Error(err)
	}
}

func TestExtractRejectsPathTraversal(t *testing.T) {
	dest := t.TempDir()
	err := ExtractTarGz(bytes.NewReader(rawTgz(t, "package/../../evil", "x")), filepath.Join(dest, "inner"))
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("want unsafe path error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "evil")); err == nil {
		t.Error("file escaped destination")
	}
}

func TestParseIntegrity(t *testing.T) {
	data := []byte("hello")
	s1 := sha1.Sum(data)
	// sha512 is preferred when several are listed.
	h, want, err := parseIntegrity("sha1-" + base64.StdEncoding.EncodeToString(s1[:]) + " " + sri(data))
	if err != nil {
		t.Fatal(err)
	}
	h.Write(data)
	if !bytes.Equal(h.Sum(nil), want) || len(want) != 64 {
		t.Error("sha512 not chosen or mismatch")
	}
	for _, bad := range []string{"", "md5-abc", "sha512-!!!"} {
		if _, _, err := parseIntegrity(bad); err == nil {
			t.Errorf("parseIntegrity(%q) should fail", bad)
		}
	}
}

func TestFromRegistryNormalises(t *testing.T) {
	m := fromRegistry(Manifest{Name: "@s/tool", Bin: []byte(`"cli.js"`),
		Dist: &Dist{Tarball: "u", Shasum: "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"}})
	if m.Resolved != "u" || m.Dist != nil || !strings.HasPrefix(m.Integrity, "sha1-") {
		t.Errorf("%+v", m)
	}
	if got := binMap(&m); got["tool"] != "cli.js" {
		t.Errorf("bin = %v", got)
	}
}

func TestPackageJSONRoundTripKeepsOrderAndFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "package.json")
	writeTestFile(t, p, `{"name":"x","version":"1.0.0","scripts":{"t":"a && b"},"dependencies":{"z":"1"},"license":"MIT"}`)
	pj, err := ReadPackageJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	pj.Dependencies["a"] = ">=1 <2"
	pj.DevDependencies = map[string]string{"d": "^1"}
	if err := pj.Write(p); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	want := `{
  "name": "x",
  "version": "1.0.0",
  "scripts": {
    "t": "a && b"
  },
  "dependencies": {
    "a": ">=1 <2",
    "z": "1"
  },
  "license": "MIT",
  "devDependencies": {
    "d": "^1"
  }
}
`
	if string(b) != want {
		t.Errorf("got\n%s\nwant\n%s", b, want)
	}
}

func TestReadPackageJSONErrors(t *testing.T) {
	if pj, err := ReadPackageJSON(filepath.Join(t.TempDir(), "nope.json")); err != nil || pj == nil {
		t.Errorf("missing file should give empty package.json, got %v", err)
	}
	p := filepath.Join(t.TempDir(), "package.json")
	writeTestFile(t, p, `{"dependencies":["not","a","map"]}`)
	if _, err := ReadPackageJSON(p); err == nil {
		t.Error("bad dependencies should fail")
	}
}

func TestLockedManifestsAndParentPath(t *testing.T) {
	l := &Lockfile{Packages: map[string]*Manifest{
		"":                              {Name: "root"},
		"node_modules/a":                {Version: "1.0.0", Resolved: "r", Dev: true},
		"node_modules/x/node_modules/a": {Version: "2.0.0", Resolved: "r"},
		"node_modules/alias":            {Name: "real", Version: "1.0.0", Resolved: "r"},
		"packages/workspace":            {Version: "1.0.0"},
	}}
	got := lockedManifests(l)
	if len(got["a"]) != 2 || len(got["real"]) != 1 || len(got) != 2 {
		t.Errorf("got %+v", got)
	}
	if got["a"][0].Dev {
		t.Error("dev flag must not leak into resolution")
	}
	if parentPath("node_modules/@s/a/node_modules/b") != "node_modules/@s/a" || parentPath("node_modules/a") != "" {
		t.Error("parentPath")
	}
}
