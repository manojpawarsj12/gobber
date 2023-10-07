package gobber

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
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
			panic(err)
		}
		version, err := v.resolvePartialVersion(packageDetails.Comparator, versionData.Versions)
		if err != nil {
			log.Fatalf("Error GetPackageData getting version data: %v", err)
		}
		return versionData.Versions[version]
	}
}
func checkIsInstalled(name string, version string, InstalledVersionsMutex map[string]string) bool {
	packageData := InstalledVersionsMutex[name]
	return packageData == version
}
func InstallPackage(packageData PackageData, InstalledVersionsMutex map[string]string) {
	if checkIsInstalled(packageData.Name, packageData.Version, InstalledVersionsMutex) {
		log.Printf("Installed %s \n", packageData.Name)
		return
	}
	log.Println("installing !!!!!", packageData.Name)
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Fatalf("Error InstallPackage getting cache dir: %v", err)
	}
	packageDestDir := filepath.Join(cacheDir, "node_cache", packageData.Name, packageData.Version)
	log.Println("Dir for installing packages !!! ", packageDestDir)
	if err := os.MkdirAll(packageDestDir, 0755); err != nil {
		log.Printf("InstallPackage: Mkdir() failed: %s", err.Error())
	}
	tarballUrl := packageData.Dist.Tarball
	ExtractTar(tarballUrl, packageDestDir)
	deps := packageData.Dependencies
	log.Println("Deps for installing packages !!! ", packageData.Name, deps)
	for name, version := range deps {
		comparator, err := v.parseSemanticVersion(version)
		if err != nil {
			log.Fatalf("Error InstallPackage parsing semantic version: %v", err)
			panic(err)
		}
		packageDetails := PackageDetails{Name: name, Comparator: comparator}
		packageData := GetPackageData(&packageDetails)
		InstallPackage(packageData, InstalledVersionsMutex)
	}
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
	log.Println("installing !!!!!", packageDetails.Name, packageDetails.Comparator)
	var InstalledVersionsMutex map[string]string
	packageData := GetPackageData(packageDetails)
	InstallPackage(packageData, InstalledVersionsMutex)
	elapsed := time.Since(start)
	log.Printf("Took %s", elapsed)
}
