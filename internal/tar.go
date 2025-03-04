package internal

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func ExtractTar(tarball_url *string, packageDestDir *string) error {
	tarRes, err := NpmGetBytes(*tarball_url)
	if err != nil {
		return fmt.Errorf("error requesting tar url: %v", err)
	}

	uncompressedStream, err := gzip.NewReader(tarRes)
	if err != nil {
		return fmt.Errorf("error creating gzip reader: %v", err)
	}
	tarReader := tar.NewReader(uncompressedStream)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("error reading tar header: %v", err)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if header.Name != "." {
				dirPath := filepath.Join(*packageDestDir, header.Name)
				if err := os.Mkdir(dirPath, 0755); err != nil {
					return fmt.Errorf("mkdir failed: %v", err)
				}
			}
		case tar.TypeReg:
			outFilePath := filepath.Join(*packageDestDir, filepath.Base(header.Name))
			if err := os.MkdirAll(filepath.Dir(outFilePath), 0755); err != nil {
				return fmt.Errorf("mkdirall failed: %v", err)
			}
			outFile, err := os.Create(outFilePath)
			if err != nil {
				return fmt.Errorf("create file failed: %v", err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return fmt.Errorf("copy file failed: %v", err)
			}
			outFile.Close()
		case tar.TypeXGlobalHeader:
			// Ignore the pax_global_header
		default:
			return fmt.Errorf("unknown type: %c in %s", header.Typeflag, header.Name)
		}
	}
	return nil
}
