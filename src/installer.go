package gobber

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

var v Versions

func GetPackageData(packageDetails *PackageDetails) PackageData {
	version := v.resolveFullVersion(packageDetails.Comparator)
	if version != "" {
		packageData, err := NpmRegistry(&packageDetails.Name)

		if err != nil {
			fmt.Println(fmt.Errorf("Error GetPackageData getting version data: %v", err))
			panic(err)
		}
		return packageData
	} else {
		versionData, err := NpmRegistryVersionData(&packageDetails.Name)
		if err != nil {
			fmt.Println(fmt.Errorf("Error GetPackageData getting version data: %v", err))
			panic(err)
		}
		version, err := v.resolvePartialVersion(packageDetails.Comparator, versionData.Versions)
		if err != nil {
			fmt.Println(fmt.Errorf("Error GetPackageData getting version data: %v", err))
			panic(err)
		}
		return versionData.Versions[version]
	}
}
func checkIsInstalled(name string, version string, InstalledVersionsMutex map[string]string) bool {
	packageData := InstalledVersionsMutex[name]
	if packageData == version {
		return true
	}
	return false
}
func InstallPackage(packageData PackageData, InstalledVersionsMutex map[string]string) {
	if checkIsInstalled(packageData.Name, packageData.Version, InstalledVersionsMutex) {
		return
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		fmt.Println(fmt.Errorf("Error InstallPackage getting cache dir: %v", err))
		panic(err)
	}
	packageDestDir := filepath.Join(cacheDir, "node_cache", packageData.Name, packageData.Version)
	fmt.Println("Dir for installing packages !!! ", packageDestDir, cacheDir)
	if err := os.Mkdir(packageDestDir, 0755); err != nil {
		log.Fatalf("InstallPackage: Mkdir() failed: %s", err.Error())
	}
	tarballUrl := packageData.Dist.Tarball
	ExtractTar(tarballUrl, packageDestDir)
	deps := packageData.Dependencies
	for name, version := range deps {
		comparator, err := v.parseSemanticVersion(version)
		if err != nil {
			fmt.Println(fmt.Errorf("Error InstallPackage parsing semantic version: %v", err))
			panic(err)
		}
		packageDetails := PackageDetails{Name: name, Comparator: comparator}
		packageData := GetPackageData(&packageDetails)
		InstallPackage(packageData, InstalledVersionsMutex)
		return
	}
}

func Parse(packageData string) (*PackageDetails, error) {
	fmt.Println("Parsing Package !!!!!", packageData)
	packageDetails, err := v.parsePackageDetails(packageData)
	if err != nil {
		return nil, fmt.Errorf("Error parsing package data: %v", err)
	}
	return packageDetails, nil
}
func Execute(packageDetails *PackageDetails) {
	fmt.Println("installing !!!!!", packageDetails.Name, packageDetails.Comparator)
	var InstalledVersionsMutex map[string]string
	packageData := GetPackageData(packageDetails)
	InstallPackage(packageData, InstalledVersionsMutex)
}
