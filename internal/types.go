package internal

type PackageData struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Dist            Dist              `json:"dist"`
}
type Dist struct {
	Tarball string `json:"tarball"`
}
type VersionsData struct {
	PackageName string                 `json:"name"`
	Versions    map[string]PackageData `json:"versions"`
}
