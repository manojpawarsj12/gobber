package gobber

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

func ExtractTarGz(gzipStream io.Reader) {

}

func ExtractTar(tarball_url string, packageDestDir string) {
	tarRes, err := NpmGetBytes(tarball_url)
	if err != nil {
		fmt.Println(fmt.Errorf("Error ExtractTar requesting tar url: %v", err))
		panic(err)
	}

	uncompressedStream, err := gzip.NewReader(tarRes)
	if err != nil {
		fmt.Println(fmt.Errorf("Error ExtractTarGz: NewReader failed: %v", err))
		panic(err)
	}
	tarReader := tar.NewReader(uncompressedStream)

	for true {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			fmt.Println(fmt.Errorf("Error ExtractTarGz:  Next() failed: %v", err))
		}
		switch header.Typeflag {
		case tar.TypeDir:
			dirPath := filepath.Join(packageDestDir, header.Name)
			if err := os.Mkdir(dirPath, 0755); err != nil {
				log.Fatalf("ExtractTarGz: Mkdir() failed: %s", err.Error())
			}
		case tar.TypeReg:
			outFilePath := filepath.Join(packageDestDir, header.Name)
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