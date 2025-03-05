package internal

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
	rawVersion = strings.TrimPrefix(rawVersion, "npm:")
	rawVersion = strings.TrimPrefix(rawVersion, "@")
	version, err := semver.NewConstraint(rawVersion)
	if err != nil {
		return nil, err
	}
	return version, nil
}

func (v *Versions) parsePackageDetails(details string) (*PackageDetails, error) {
	if strings.HasPrefix(details, "@") {
		lastAtIndex := strings.LastIndex(details, "@")

		if lastAtIndex == 0 {
			return &PackageDetails{
				Name:       details,
				Comparator: nil,
			}, nil
		}

		name := details[:lastAtIndex]
		version := details[lastAtIndex+1:]

		if version == latest || version == "" {
			return &PackageDetails{
				Name:       name,
				Comparator: nil,
			}, nil
		}

		comparator, err := v.parseSemanticVersion(version)
		if err != nil {
			return nil, err
		}
		return &PackageDetails{
			Name:       name,
			Comparator: comparator,
		}, nil
	}

	parts := strings.Split(details, "@")

	if len(parts) == 1 || parts[1] == latest || parts[1] == "" {
		return &PackageDetails{
			Name:       parts[0],
			Comparator: nil,
		}, nil
	}

	comparator, err := v.parseSemanticVersion(parts[1])
	if err != nil {
		return nil, err
	}

	return &PackageDetails{
		Name:       parts[0],
		Comparator: comparator,
	}, nil
}

func (v *Versions) resolveFullVersion(semanticVersion *semver.Constraints) string {
	if semanticVersion == nil {
		return latest
	}

	constraint := (*semanticVersion).String()
	if len(constraint) < 3 {
		return latest
	}

	// Handle the constraint format properly
	parts := strings.Split(constraint, ".")
	if len(parts) < 3 {
		return latest
	}

	major := parts[0]
	minor := parts[1]
	patch := parts[2]

	switch string(constraint[0]) {
	case ">":
		return fmt.Sprintf(">%s.%s.%s", major, minor, patch)
	case "=", "~", "^":
		return v.toString(major, minor, patch)
	default:
		return latest
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
