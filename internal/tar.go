package internal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ExtractTarGz unpacks an npm tarball into dest, dropping the top-level
// directory ("package/" in almost every tarball). Symlinks and other special
// entries are skipped, as npm does.
func ExtractTarGz(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		name := path.Clean(strings.TrimPrefix(h.Name, "./"))
		_, rel, ok := strings.Cut(name, "/")
		if !ok || rel == "" {
			continue
		}
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("tar: unsafe path %q", h.Name)
		}
		p := filepath.Join(dest, rel)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(p, tr, h.Mode); err != nil {
				return err
			}
		}
	}
}

func writeFile(p string, r io.Reader, mode int64) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	perm := os.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// extractVerified extracts while hashing the raw tarball, then checks it against an SRI string.
func extractVerified(r io.Reader, integrity, dest string) error {
	h, want, err := parseIntegrity(integrity)
	if err != nil {
		return err
	}
	tee := io.TeeReader(r, h)
	if err := ExtractTarGz(tee, dest); err != nil {
		return err
	}
	if _, err := io.Copy(io.Discard, tee); err != nil {
		return err
	}
	if !bytes.Equal(h.Sum(nil), want) {
		return fmt.Errorf("integrity check failed (expected %s)", integrity)
	}
	return nil
}

// parseIntegrity picks the strongest supported hash from an SRI string like "sha512-<base64>".
func parseIntegrity(sri string) (hash.Hash, []byte, error) {
	algos := []struct {
		prefix string
		new    func() hash.Hash
	}{{"sha512-", sha512.New}, {"sha384-", sha512.New384}, {"sha256-", sha256.New}, {"sha1-", sha1.New}}
	fields := strings.Fields(sri)
	for _, a := range algos {
		for _, f := range fields {
			if !strings.HasPrefix(f, a.prefix) {
				continue
			}
			b64, _, _ := strings.Cut(f[len(a.prefix):], "?")
			want, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				return nil, nil, fmt.Errorf("bad integrity %q: %w", f, err)
			}
			return a.new(), want, nil
		}
	}
	return nil, nil, fmt.Errorf("missing or unsupported integrity %q", sri)
}
