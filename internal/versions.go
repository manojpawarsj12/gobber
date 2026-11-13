package internal

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

// ParseArg splits "react@^18", "@types/node@20" or "alias@npm:real@1" into name and spec.
func ParseArg(arg string) (name, spec string) {
	if len(arg) > 1 {
		if i := strings.Index(arg[1:], "@"); i >= 0 {
			return arg[:i+1], arg[i+2:]
		}
	}
	return arg, ""
}

// Names end up in filesystem paths, so they are validated (no "..", no separators beyond the scope).
var nameRe = regexp.MustCompile(`^(?:@[A-Za-z0-9~-][\w.~-]*/)?[A-Za-z0-9~-][\w.~-]*$`)

func validName(name string) bool { return nameRe.MatchString(name) }

// saveSpec mirrors npm: tags, exact versions, "*" and ranges that contain ^version
// are saved as "^version"; any other range is saved as typed.
func saveSpec(spec, version string) string {
	if strings.HasPrefix(spec, "npm:") {
		return spec
	}
	prefixed := "^" + version
	r, err := parseRange(spec)
	if strings.TrimSpace(spec) == "" || err != nil || strings.TrimSpace(spec) == "*" {
		return prefixed
	}
	if _, err := parseVersion(spec); err == nil {
		return prefixed
	}
	if v, err := parseVersion(version); err == nil && caretSubset(r, v) {
		return prefixed
	}
	return spec
}

// pickVersion follows npm-pick-manifest: a dist-tag wins, then "latest" if it
// satisfies the range, then the highest non-deprecated match, then any match.
func pickVersion(p *Packument, spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		spec = "latest"
	}
	if v, ok := p.DistTags[spec]; ok {
		if _, ok := p.Versions[v]; ok {
			return v, nil
		}
	}
	r, err := parseRange(spec)
	if err != nil {
		return "", fmt.Errorf("unsupported version spec %q (git, file and url specs are not supported)", spec)
	}
	if l, ok := p.DistTags["latest"]; ok {
		if _, ok := p.Versions[l]; ok && (spec == "*" || satisfies(spec, l)) {
			return l, nil
		}
	}
	var live, all []string
	for v, m := range p.Versions {
		all = append(all, v)
		if !m.deprecated() {
			live = append(live, v)
		}
	}
	if v := highest(r, live); v != "" {
		return v, nil
	}
	if v := highest(r, all); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("no version of %s matches %q", p.Name, spec)
}

// pickLocked returns the highest lockfile entry satisfying spec. Tags (and "", meaning latest) go to the registry.
func pickLocked(cands []Manifest, spec string) (Manifest, bool) {
	if strings.TrimSpace(spec) == "" || len(cands) == 0 {
		return Manifest{}, false
	}
	r, err := parseRange(spec)
	if err != nil {
		return Manifest{}, false
	}
	versions := make([]string, len(cands))
	for i, m := range cands {
		versions[i] = m.Version
	}
	best := highest(r, versions)
	for _, m := range cands {
		if best != "" && m.Version == best {
			return m, true
		}
	}
	return Manifest{}, false
}

func highest(r semverRange, versions []string) string {
	var best *version
	bestRaw := ""
	for _, raw := range versions {
		v, err := parseVersion(raw)
		if err != nil || !r.test(v) {
			continue
		}
		if best == nil || compareVersions(v, best) > 0 {
			best, bestRaw = v, raw
		}
	}
	return bestRaw
}

// platformOK applies the package's "os"/"cpu" fields (with "!" negation) using node's names.
func platformOK(m *Manifest) bool {
	os, arch := runtime.GOOS, runtime.GOARCH
	if os == "windows" {
		os = "win32"
	}
	switch arch {
	case "amd64":
		arch = "x64"
	case "386":
		arch = "ia32"
	}
	return allowed(m.OS, os) && allowed(m.CPU, arch)
}

func allowed(list []string, v string) bool {
	match, positive := false, false
	for _, x := range list {
		if strings.HasPrefix(x, "!") {
			if x[1:] == v {
				return false
			}
			continue
		}
		positive = true
		match = match || x == v
	}
	return match || !positive
}
