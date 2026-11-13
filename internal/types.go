package internal

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"path"
)

// Manifest is one package version. The same shape is used for registry
// metadata and for "packages" entries in package-lock.json.
type Manifest struct {
	Name                 string              `json:"name,omitempty"`
	Version              string              `json:"version,omitempty"`
	Resolved             string              `json:"resolved,omitempty"`
	Integrity            string              `json:"integrity,omitempty"`
	Dev                  bool                `json:"dev,omitempty"`
	Dependencies         map[string]string   `json:"dependencies,omitempty"`
	DevDependencies      map[string]string   `json:"devDependencies,omitempty"`
	OptionalDependencies map[string]string   `json:"optionalDependencies,omitempty"`
	PeerDependencies     map[string]string   `json:"peerDependencies,omitempty"`
	PeerDependenciesMeta map[string]PeerMeta `json:"peerDependenciesMeta,omitempty"`
	Bin                  json.RawMessage     `json:"bin,omitempty"`
	OS                   []string            `json:"os,omitempty"`
	CPU                  []string            `json:"cpu,omitempty"`
	Dist                 *Dist               `json:"dist,omitempty"`
	// Raw because some old packuments have "deprecated": false; npm doesn't keep it in the lockfile.
	Deprecated json.RawMessage `json:"deprecated,omitempty"`
}

func (m *Manifest) deprecated() bool {
	var s string
	return json.Unmarshal(m.Deprecated, &s) == nil && s != ""
}

type PeerMeta struct {
	Optional bool `json:"optional,omitempty"`
}

type Dist struct {
	Tarball   string `json:"tarball"`
	Integrity string `json:"integrity,omitempty"`
	Shasum    string `json:"shasum,omitempty"`
}

// Packument is the (abbreviated) registry document listing every version of a package.
type Packument struct {
	Name     string              `json:"name"`
	DistTags map[string]string   `json:"dist-tags"`
	Versions map[string]Manifest `json:"versions"`
}

// binMap normalises "bin" which npm allows as a string or a name->path object.
func binMap(m *Manifest) map[string]string {
	var s string
	if json.Unmarshal(m.Bin, &s) == nil && s != "" {
		return map[string]string{path.Base(m.Name): s}
	}
	var out map[string]string
	_ = json.Unmarshal(m.Bin, &out)
	return out
}

// fromRegistry turns registry metadata into the lockfile shape.
func fromRegistry(m Manifest) Manifest {
	if m.Dist != nil {
		m.Resolved = m.Dist.Tarball
		m.Integrity = m.Dist.Integrity
		if m.Integrity == "" && m.Dist.Shasum != "" {
			if b, err := hex.DecodeString(m.Dist.Shasum); err == nil {
				m.Integrity = "sha1-" + base64.StdEncoding.EncodeToString(b)
			}
		}
	}
	m.Dist = nil
	m.Deprecated = nil
	m.DevDependencies = nil
	bins := binMap(&m)
	m.Bin = nil
	if len(bins) > 0 {
		m.Bin, _ = json.Marshal(bins)
	}
	return m
}
