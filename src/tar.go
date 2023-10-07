package gobber

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var wgTar sync.WaitGroup

func Extract(tarball_url *string, packageDestDir *string) {
	wgTar.Add(1)
	go ExtractTar(tarball_url, packageDestDir)
	wgTar.Wait()
}

func ExtractTar(tarball_url *string, packageDestDir *string) {
	defer wgTar.Done()
	tarRes, err := NpmGetBytes(*tarball_url)
	if err != nil {
		log.Fatalf("Error ExtractTar requesting tar url: %v", err)
	}

	uncompressedStream, err := gzip.NewReader(tarRes)
	if err != nil {
		log.Fatalf("Error ExtractTarGz: NewReader failed: %v", err)
	}
	tarReader := tar.NewReader(uncompressedStream)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			log.Fatalf("Error ExtractTarGz:  Next() failed: %v", err)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			dirPath := filepath.Join(*packageDestDir, header.Name)
			if err := os.Mkdir(dirPath, 0755); err != nil {
				log.Fatalf("ExtractTarGz: Mkdir() failed: %s", err.Error())
			}
		case tar.TypeReg:
			outFilePath := filepath.Join(*packageDestDir, header.Name)
			if err := os.MkdirAll(filepath.Dir(outFilePath), 0755); err != nil {
				log.Fatalf("ExtractTarGz: MkdirAll() failed: %s", err.Error())
			}
			outFile, err := os.Create(outFilePath)
			if err != nil {
				log.Fatalf("ExtractTarGz: Create() failed: %s", err.Error())
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				log.Fatalf("ExtractTarGz: Copy() failed: %s", err.Error())
			}
			outFile.Close()

		default:
			log.Fatalf(
				"ExtractTarGz: uknown type: %s in %s",
				header.Typeflag,
				header.Name)
		}

	}

}
