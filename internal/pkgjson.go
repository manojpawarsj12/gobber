package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// PackageJSON edits dependencies while keeping every other field, and the key order, untouched.
type PackageJSON struct {
	keys            []string
	fields          map[string]json.RawMessage
	Name            string
	Version         string
	Dependencies    map[string]string
	DevDependencies map[string]string
}

// ReadPackageJSON returns an empty PackageJSON when the file does not exist.
func ReadPackageJSON(file string) (*PackageJSON, error) {
	p := &PackageJSON{fields: map[string]json.RawMessage{}}
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, fmt.Errorf("%s: expected a JSON object", file)
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		key := t.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if _, dup := p.fields[key]; !dup {
			p.keys = append(p.keys, key)
		}
		p.fields[key] = raw
	}
	for key, dst := range map[string]any{"name": &p.Name, "version": &p.Version,
		"dependencies": &p.Dependencies, "devDependencies": &p.DevDependencies} {
		if raw, ok := p.fields[key]; ok {
			if err := json.Unmarshal(raw, dst); err != nil {
				return nil, fmt.Errorf("%s: field %q: %w", file, key, err)
			}
		}
	}
	return p, nil
}

// shortcut: always writes 2-space indentation; detect the file's indent if users complain.
func (p *PackageJSON) Write(file string) error {
	for key, m := range map[string]map[string]string{"dependencies": p.Dependencies, "devDependencies": p.DevDependencies} {
		if _, ok := p.fields[key]; !ok && len(m) == 0 {
			continue
		}
		raw, err := marshal(m, "")
		if err != nil {
			return err
		}
		if _, ok := p.fields[key]; !ok {
			p.keys = append(p.keys, key)
		}
		p.fields[key] = raw
	}
	var b bytes.Buffer
	b.WriteString("{")
	for i, k := range p.keys {
		if i > 0 {
			b.WriteString(",")
		}
		kb, _ := marshal(k, "")
		b.WriteString("\n  ")
		b.Write(kb)
		b.WriteString(": ")
		if err := json.Indent(&b, p.fields[k], "  ", "  "); err != nil {
			return err
		}
	}
	b.WriteString("\n}\n")
	return os.WriteFile(file, b.Bytes(), 0o644)
}

// marshal without HTML escaping, so ranges like ">=1 <2" stay readable.
func marshal(v any, indent string) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", indent)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}
