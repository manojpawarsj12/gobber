package gobber

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const latest = "latest"

type PackageDetails struct {
	Name       string
	Comparator *semver.Constraints
}

type Versions struct{}

func (v *Versions) parseSemanticVersion(rawVersion string) (*semver.Constraints, error) {
	version, err := semver.NewConstraint(rawVersion)
	if err != nil {
		return nil, err
	}
	return version, nil
}

func (v *Versions) parsePackageDetails(details string) (*PackageDetails, error) {
	parts := strings.Split(details, "@")

	name := parts[0]

	if len(parts) == 1 || parts[1] == latest {
		return &PackageDetails{Name: name, Comparator: nil}, nil
	}

	comparator, err := v.parseSemanticVersion(parts[1])
	if err != nil {
		return nil, err
	}

	return &PackageDetails{Name: name, Comparator: comparator}, nil
}

func (v *Versions) resolveFullVersion(semanticVersion *semver.Constraints) string {
	if semanticVersion == nil {
		return latest
	}

	constraint := (*semanticVersion).String()
	if len(constraint) < 3 {
		return latest
	}
	parts := strings.Split(constraint, ".")
	major := parts[0]
	minor := parts[1]
	patch := parts[2]

	switch string(constraint[0]) {
	case ">":
		return fmt.Sprintf(">%s.%s.%s", major, minor, patch)
	case "=", "~", "^":
		return v.toString(major, minor, patch)
	default:
		return ""
	}
}

func (v *Versions) resolvePartialVersion(semanticVersion *semver.Constraints, availableVersions map[string]PackageData) (string, error) {
	if semanticVersion == nil {
		return "", fmt.Errorf("semantic version is nil")
	}

	var versions []*semver.Version
	for version := range availableVersions {
		v, _ := semver.NewVersion(version)
		versions = append(versions, v)
	}
	sort.Sort(semver.Collection(versions))
	for i := len(versions) - 1; i >= 0; i-- {
		version := versions[i]

		if (*semanticVersion).Check(version) {
			return version.String(), nil
		}
	}

	return "", fmt.Errorf("invalid version")
}
func (v *Versions) toString(major string, minor string, patch string) string {
	return fmt.Sprintf("%s.%s.%s", major, minor, patch)
}
