package internal

// npm-compatible semver: a port of node-semver's range rules (strict mode),
// so ranges match exactly the versions `npm install` would accept.

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type version struct {
	major, minor, patch uint64
	pre                 []string
}

var (
	versionRe = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
		`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
		`(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
	partialRe = regexp.MustCompile(`^v?(\d+|[xX*])(?:\.(\d+|[xX*])(?:\.(\d+|[xX*])` +
		`(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?)?)?$`)
	hyphenRe  = regexp.MustCompile(`^(\S+)\s+-\s+(\S+)$`)
	opSpaceRe = regexp.MustCompile(`(<=|>=|<|>|=|~>|~|\^)\s+`)
)

// parseVersion accepts strict semver, as npm does for registry versions ("1.2" is not a version).
func parseVersion(s string) (*version, error) {
	m := versionRe.FindStringSubmatch(strings.TrimPrefix(strings.TrimSpace(s), "="))
	if m == nil {
		return nil, fmt.Errorf("invalid version %q", s)
	}
	v := &version{}
	for i, dst := range []*uint64{&v.major, &v.minor, &v.patch} {
		n, err := strconv.ParseUint(m[i+1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid version %q", s)
		}
		*dst = n
	}
	if m[4] != "" {
		v.pre = strings.Split(m[4], ".")
	}
	return v, nil
}

func compareVersions(a, b *version) int {
	for _, p := range [][2]uint64{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if p[0] != p[1] {
			if p[0] < p[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1 // a release sorts after its prereleases
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := compareIdent(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(a.pre), len(b.pre))
}

func compareIdent(a, b string) int {
	an, aErr := strconv.ParseUint(a, 10, 64)
	bn, bErr := strconv.ParseUint(b, 10, 64)
	switch {
	case aErr == nil && bErr == nil:
		if an == bn {
			return 0
		}
		if an < bn {
			return -1
		}
		return 1
	case aErr == nil:
		return -1 // numeric identifiers sort before alphanumeric ones
	case bErr == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// comparator with op "" matches any version.
type comparator struct {
	op string
	v  *version
}

// semverRange is OR-of-ANDs, like node-semver's Range.set.
type semverRange [][]comparator

var anyVersion = []comparator{{}}

func parseRange(spec string) (semverRange, error) {
	var r semverRange
	for _, part := range strings.Split(spec, "||") {
		set, err := parseSet(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("invalid range %q: %w", spec, err)
		}
		r = append(r, set)
	}
	return r, nil
}

func parseSet(s string) ([]comparator, error) {
	if m := hyphenRe.FindStringSubmatch(s); m != nil {
		return hyphen(m[1], m[2])
	}
	s = opSpaceRe.ReplaceAllString(s, "$1")
	var out []comparator
	for _, tok := range strings.Fields(s) {
		cs, err := desugar(tok)
		if err != nil {
			return nil, err
		}
		out = append(out, cs...)
	}
	if len(out) == 0 {
		return anyVersion, nil // "" means "*"
	}
	return out, nil
}

// partial is a possibly incomplete version; -1 marks x/X/* or a missing part.
type partial struct {
	M, m, p int64
	pre     string
}

func parsePartial(s string) (partial, error) {
	mt := partialRe.FindStringSubmatch(s)
	if mt == nil {
		return partial{}, fmt.Errorf("bad version %q", s)
	}
	nums := [3]int64{-1, -1, -1}
	for i := 0; i < 3; i++ {
		if mt[i+1] == "" || strings.ContainsAny(mt[i+1], "xX*") {
			break // a wildcard makes every later part a wildcard too
		}
		n, err := strconv.ParseInt(mt[i+1], 10, 64)
		if err != nil || n == math.MaxInt64 {
			return partial{}, fmt.Errorf("bad version %q", s)
		}
		nums[i] = n
	}
	pt := partial{M: nums[0], m: nums[1], p: nums[2]}
	if pt.p >= 0 {
		pt.pre = mt[4]
	}
	return pt, nil
}

func ver(M, m, p int64, pre string) *version {
	v := &version{major: uint64(M), minor: uint64(m), patch: uint64(p)}
	if pre != "" {
		v.pre = strings.Split(pre, ".")
	}
	return v
}

func between(lo, hi *version) []comparator {
	return []comparator{{">=", lo}, {"<", hi}}
}

func desugar(tok string) ([]comparator, error) {
	op := ""
	for _, o := range []string{"<=", ">=", "~>", "<", ">", "=", "~", "^"} {
		if strings.HasPrefix(tok, o) {
			op = o
			break
		}
	}
	pt, err := parsePartial(tok[len(op):])
	if err != nil {
		return nil, err
	}
	M, m, p := pt.M, pt.m, pt.p
	switch op {
	case "^":
		switch {
		case M < 0:
			return anyVersion, nil
		case m < 0:
			return between(ver(M, 0, 0, ""), ver(M+1, 0, 0, "0")), nil
		case p < 0 && M == 0:
			return between(ver(0, m, 0, ""), ver(0, m+1, 0, "0")), nil
		case p < 0:
			return between(ver(M, m, 0, ""), ver(M+1, 0, 0, "0")), nil
		case M == 0 && m == 0:
			return between(ver(0, 0, p, pt.pre), ver(0, 0, p+1, "0")), nil
		case M == 0:
			return between(ver(0, m, p, pt.pre), ver(0, m+1, 0, "0")), nil
		}
		return between(ver(M, m, p, pt.pre), ver(M+1, 0, 0, "0")), nil
	case "~", "~>":
		switch {
		case M < 0:
			return anyVersion, nil
		case m < 0:
			return between(ver(M, 0, 0, ""), ver(M+1, 0, 0, "0")), nil
		case p < 0:
			return between(ver(M, m, 0, ""), ver(M, m+1, 0, "0")), nil
		}
		return between(ver(M, m, p, pt.pre), ver(M, m+1, 0, "0")), nil
	case "", "=":
		switch {
		case M < 0:
			return anyVersion, nil
		case m < 0:
			return between(ver(M, 0, 0, ""), ver(M+1, 0, 0, "0")), nil
		case p < 0:
			return between(ver(M, m, 0, ""), ver(M, m+1, 0, "0")), nil
		}
		return []comparator{{"=", ver(M, m, p, pt.pre)}}, nil
	}
	// <, <=, >, >= with a partial version, following node-semver's replaceXRange.
	if M < 0 {
		if op == "<" || op == ">" {
			return []comparator{{"<", ver(0, 0, 0, "0")}}, nil // matches nothing
		}
		return anyVersion, nil
	}
	if p >= 0 {
		return []comparator{{op, ver(M, m, p, pt.pre)}}, nil
	}
	xm := m < 0
	if xm {
		m = 0
	}
	p = 0
	switch op {
	case ">":
		op = ">="
		if xm {
			M, m = M+1, 0
		} else {
			m++
		}
	case "<=":
		op = "<"
		if xm {
			M++
		} else {
			m++
		}
	}
	pre := ""
	if op == "<" {
		pre = "0"
	}
	return []comparator{{op, ver(M, m, p, pre)}}, nil
}

func hyphen(from, to string) ([]comparator, error) {
	f, err := parsePartial(from)
	if err != nil {
		return nil, err
	}
	t, err := parsePartial(to)
	if err != nil {
		return nil, err
	}
	var out []comparator
	switch {
	case f.M < 0:
	case f.m < 0:
		out = append(out, comparator{">=", ver(f.M, 0, 0, "")})
	case f.p < 0:
		out = append(out, comparator{">=", ver(f.M, f.m, 0, "")})
	default:
		out = append(out, comparator{">=", ver(f.M, f.m, f.p, f.pre)})
	}
	switch {
	case t.M < 0:
	case t.m < 0:
		out = append(out, comparator{"<", ver(t.M+1, 0, 0, "0")})
	case t.p < 0:
		out = append(out, comparator{"<", ver(t.M, t.m+1, 0, "0")})
	default:
		out = append(out, comparator{"<=", ver(t.M, t.m, t.p, t.pre)})
	}
	if len(out) == 0 {
		return anyVersion, nil
	}
	return out, nil
}

func (r semverRange) test(v *version) bool {
	for _, set := range r {
		if testSet(set, v) {
			return true
		}
	}
	return false
}

func testSet(set []comparator, v *version) bool {
	for _, c := range set {
		if c.v == nil {
			continue
		}
		cmp := compareVersions(v, c.v)
		ok := map[string]bool{"=": cmp == 0, "<": cmp < 0, "<=": cmp <= 0, ">": cmp > 0, ">=": cmp >= 0}[c.op]
		if !ok {
			return false
		}
	}
	if len(v.pre) == 0 {
		return true
	}
	// npm rule: a prerelease only matches if some comparator names a prerelease of the same x.y.z.
	for _, c := range set {
		if c.v != nil && len(c.v.pre) > 0 && c.v.major == v.major && c.v.minor == v.minor && c.v.patch == v.patch {
			return true
		}
	}
	return false
}

func satisfies(spec, v string) bool {
	r, err := parseRange(spec)
	if err != nil {
		return false
	}
	pv, err := parseVersion(v)
	return err == nil && r.test(pv)
}

// caretSubset reports whether ^v fits entirely inside r; npm then saves "^v" instead of the typed range.
// shortcut: probes the bounds of ^v instead of a full range-subset check; ranges with holes inside ^v may save as ^v.
func caretSubset(r semverRange, v *version) bool {
	if len(v.pre) > 0 {
		return false
	}
	var top *version
	switch {
	case v.major > 0:
		top = &version{major: v.major, minor: math.MaxUint64, patch: math.MaxUint64}
	case v.minor > 0:
		top = &version{minor: v.minor, patch: math.MaxUint64}
	default:
		top = v
	}
	return r.test(v) && r.test(top)
}
