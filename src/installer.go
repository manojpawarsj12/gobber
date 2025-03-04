package gobber

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

func checkIsInstalled(name string, version string, InstalledVersionsMutex *map[string]string, mut *sync.Mutex) bool {
	mut.Lock()
	defer mut.Unlock()
	packageData := (*InstalledVersionsMutex)[name]
	return packageData == version
}

func ancestorInstalled(name string, projectDir string, InstalledVersionsMutex *map[string]string, mut *sync.Mutex) bool {
	dir := projectDir
	for {
		mut.Lock()
		_, isInstalled := (*InstalledVersionsMutex)[name]
		mut.Unlock()
		if isInstalled {
			return true
		}
		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			break
		}
		dir = parentDir
	}
	return false
}

func InstallPackageRecursive(packageData PackageData, InstalledVersionsMutex *map[string]string, projectDir string, mut *sync.Mutex) error {
	if checkIsInstalled(packageData.Name, packageData.Version, InstalledVersionsMutex, mut) {
		log.Printf("Installed %s \n", packageData.Name)
		return nil
	}
	log.Println("installing !!!!!", packageData.Name,packageData.Version)

	if ancestorInstalled(packageData.Name, projectDir, InstalledVersionsMutex, mut) {
		log.Printf("%s is already installed at an ancestor node_modules folder\n", packageData.Name)
		return nil
	}

	// Create a directory for the package in the node_modules folder
	packageDestDir := filepath.Join(projectDir, "node_modules", packageData.Name)
	if err := os.MkdirAll(packageDestDir, 0755); err != nil {
		return fmt.Errorf("mkdir failed: %v", err)
	}

	extractionDone := make(chan struct{})

	go func() {
		defer close(extractionDone)

		// Extract the tarball to the package directory
		tarballUrl := packageData.Dist.Tarball
		if err := ExtractTar(&tarballUrl, &packageDestDir); err != nil {
			log.Printf("error extracting tarball: %v", err)
		}

		mut.Lock()
		(*InstalledVersionsMutex)[packageData.Name] = packageData.Version
		mut.Unlock()
	}()
	done := make(chan bool)
	deps := packageData.Dependencies
	log.Println("Deps for installing packages !!! ", packageData.Name, deps)

	for name, version := range deps {
		comparator, err := v.parseSemanticVersion(version)
		if err != nil {
			return fmt.Errorf("error parsing semantic version: %v", err)
		}
		go func(name, version string) {
			packageDetails := PackageDetails{Name: name, Comparator: comparator}
			depPackageData, err := GetPackageData(&packageDetails)
			if err != nil {
				log.Printf("error getting package data: %v", err)
				done <- false
				return
			}
			// Install dependencies recursively inside the packageDestDir
			if err := InstallPackageRecursive(depPackageData, InstalledVersionsMutex, packageDestDir, mut); err != nil {
				log.Printf("error installing package recursively: %v", err)
				done <- false
				return
			}
			done <- true
		}(name, version)
	}

	for range deps {
		// Wait for InstallPackageRecursive to complete
		if !<-done {
			return fmt.Errorf("error installing dependencies")
		}
	}

	// Wait for tarball extraction to complete
	<-extractionDone
	return nil
}

func Execute(packageDetails *PackageDetails, mut *sync.Mutex, InstalledVersionsMutex *map[string]string, projectDir string) error {
	done := make(chan bool)

	go func() {
		packageData, err := GetPackageData(packageDetails)
		if err != nil {
			log.Printf("error getting package data: %v", err)
			done <- false
			return
		}
		if err := InstallPackageRecursive(packageData, InstalledVersionsMutex, projectDir, mut); err != nil {
			log.Printf("error installing package recursively: %v", err)
			done <- false
			return
		}
		done <- true
	}()

	if !<-done {
		return fmt.Errorf("error executing package installation")
	}
	return nil
}
