package internal

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/Masterminds/semver/v3"
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

// saveSpec is what gets written to package.json for an added package.
func saveSpec(spec, version string) string {
	if strings.HasPrefix(spec, "npm:") {
		return spec
	}
	if spec != "" {
		if _, err := semver.NewConstraint(spec); err == nil {
			return spec
		}
	}
	return "^" + version
}

// pickVersion chooses like npm: a dist-tag wins, then "latest" if it satisfies, then the highest match.
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
	c, err := semver.NewConstraint(spec)
	if err != nil {
		return "", fmt.Errorf("unsupported version spec %q (git, file and url specs are not supported)", spec)
	}
	if l, ok := p.DistTags["latest"]; ok {
		if _, ok := p.Versions[l]; ok && satisfies(c, l) {
			return l, nil
		}
	}
	versions := make([]string, 0, len(p.Versions))
	for v := range p.Versions {
		versions = append(versions, v)
	}
	if v := highest(c, versions); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("no version of %s matches %q", p.Name, spec)
}

// pickLocked returns the highest lockfile entry satisfying spec. Dist-tags never match the lockfile.
func pickLocked(cands []Manifest, spec string) (Manifest, bool) {
	c, err := semver.NewConstraint(spec)
	if err != nil || len(cands) == 0 {
		return Manifest{}, false
	}
	versions := make([]string, len(cands))
	for i, m := range cands {
		versions[i] = m.Version
	}
	best := highest(c, versions)
	for _, m := range cands {
		if m.Version == best && best != "" {
			return m, true
		}
	}
	return Manifest{}, false
}

func highest(c *semver.Constraints, versions []string) string {
	var best *semver.Version
	bestRaw := ""
	for _, raw := range versions {
		v, err := semver.NewVersion(raw)
		if err != nil || !c.Check(v) {
			continue
		}
		if best == nil || v.GreaterThan(best) {
			best, bestRaw = v, raw
		}
	}
	return bestRaw
}

func satisfies(c *semver.Constraints, version string) bool {
	v, err := semver.NewVersion(version)
	return err == nil && c.Check(v)
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
