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
	defer tarRes.Close()

	uncompressedStream, err := gzip.NewReader(tarRes)
	if err != nil {
		return fmt.Errorf("error creating gzip reader: %v", err)
	}
	defer uncompressedStream.Close()

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
				if _, err := os.Stat(dirPath); os.IsNotExist(err) {
					if err := os.MkdirAll(dirPath, 0755); err != nil {
						return fmt.Errorf("mkdir failed for %s: %v", dirPath, err)
					}
				}
			}
		case tar.TypeReg:
			outFilePath := filepath.Join(*packageDestDir, header.Name)
			// Make sure the directory exists
			if err := os.MkdirAll(filepath.Dir(outFilePath), 0755); err != nil {
				return fmt.Errorf("mkdirall failed for %s: %v", filepath.Dir(outFilePath), err)
			}
			outFile, err := os.Create(outFilePath)
			if err != nil {
				return fmt.Errorf("create file failed for %s: %v", outFilePath, err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("copy file failed for %s: %v", outFilePath, err)
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
