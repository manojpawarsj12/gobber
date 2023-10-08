package gobber

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var v Versions

func GetPackageData(packageDetails *PackageDetails) PackageData {
	version := v.resolveFullVersion(packageDetails.Comparator)
	if version != "" {
		packageData, err := NpmRegistry(&packageDetails.Name)

		if err != nil {
			log.Fatalf("Error GetPackageData getting version data: %v", err)
		}
		return packageData
	} else {
		versionData, err := NpmRegistryVersionData(&packageDetails.Name)
		if err != nil {
			log.Fatalf("Error GetPackageData getting version data: %v", err)
		}
		version, err := v.resolvePartialVersion(packageDetails.Comparator, versionData.Versions)
		if err != nil {
			log.Fatalf("Error GetPackageData getting version data: %v", err)
		}
		return versionData.Versions[version]
	}
}
func checkIsInstalled(name string, version string, InstalledVersionsMutex *map[string]string, mut *sync.Mutex) bool {
	mut.Lock()
	packageData := (*InstalledVersionsMutex)[name]
	mut.Unlock()
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
func InstallPackage(packageData PackageData, InstalledVersionsMutex *map[string]string, projectDir string, mut *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	if checkIsInstalled(packageData.Name, packageData.Version, InstalledVersionsMutex, mut) {
		log.Printf("Installed %s \n", packageData.Name)
		return
	}
	log.Println("installing !!!!!", packageData.Name)

	if ancestorInstalled(packageData.Name, projectDir, InstalledVersionsMutex, mut) {
		log.Printf("%s is already installed at an ancestor node_modules folder\n", packageData.Name)
		return
	}

	// Create a directory for the package in the node_modules folder
	packageDestDir := filepath.Join(projectDir, "node_modules", packageData.Name)
	if err := os.MkdirAll(packageDestDir, 0755); err != nil {
		log.Printf("InstallPackage: Mkdir() failed: %s", err.Error())
	}

	extractionDone := make(chan struct{})

	go func() {
		defer close(extractionDone)

		// Extract the tarball to the package directory
		tarballUrl := packageData.Dist.Tarball
		ExtractTar(&tarballUrl, &packageDestDir)

		mut.Lock()
		(*InstalledVersionsMutex)[packageData.Name] = packageData.Version
		mut.Unlock()
	}()
	var depWG sync.WaitGroup

	deps := packageData.Dependencies
	log.Println("Deps for installing packages !!! ", packageData.Name, deps)

	for name, version := range deps {
		comparator, err := v.parseSemanticVersion(version)
		if err != nil {
			log.Fatalf("Error InstallPackage parsing semantic version: %v", err)
		}
		depWG.Add(1)
		go func(name, version string) {
			defer depWG.Done()
			packageDetails := PackageDetails{Name: name, Comparator: comparator}
			depPackageData := GetPackageData(&packageDetails)
			// Install dependencies recursively inside the packageDestDir
			wg.Add(1)
			InstallPackage(depPackageData, InstalledVersionsMutex, packageDestDir, mut, wg)
		}(name, version)
	}
	// Wait for dependency installation to complete
	depWG.Wait()

	// Wait for tarball extraction to complete
	<-extractionDone
}

func Parse(packageData string) (*PackageDetails, error) {
	log.Println("Parsing Package !!!!!", packageData)
	packageDetails, err := v.parsePackageDetails(packageData)
	if err != nil {
		return nil, fmt.Errorf("Error parsing package data: %v", err)
	}
	return packageDetails, nil
}
func Execute(packageDetails *PackageDetails) {
	start := time.Now()
	var mut sync.Mutex
	var wg sync.WaitGroup
	var InstalledVersionsMutex = make(map[string]string)
	wd, _ := os.Getwd()
	packageData := GetPackageData(packageDetails)
	wg.Add(1)
	go InstallPackage(packageData, &InstalledVersionsMutex, wd, &mut, &wg)
	wg.Wait()
	elapsed := time.Since(start)
	log.Printf("Took %s", elapsed)
	log.Println("Installed total packages: ", len(InstalledVersionsMutex))
}
