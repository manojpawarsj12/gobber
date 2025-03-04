package internal

import (
	"fmt"
	"log"
)

var v Versions

func GetPackageData(packageDetails *PackageDetails) (PackageData, error) {
	version := v.resolveFullVersion(packageDetails.Comparator)
	if version != "" {
		packageData, err := NpmRegistry(&packageDetails.Name)
		if err != nil {
			return PackageData{}, fmt.Errorf("error getting version data: %v", err)
		}
		return packageData, nil
	} else {
		versionData, err := NpmRegistryVersionData(&packageDetails.Name)
		if err != nil {
			return PackageData{}, fmt.Errorf("error getting version data: %v", err)
		}
		version, err := v.resolvePartialVersion(packageDetails.Comparator, versionData.Versions)
		if err != nil {
			return PackageData{}, fmt.Errorf("error resolving partial version: %v", err)
		}
		return versionData.Versions[version], nil
	}
}

func Parse(packageData string) (*PackageDetails, error) {
	log.Println("Parsing Package !!!!!", packageData)
	packageDetails, err := v.parsePackageDetails(packageData)
	if err != nil {
		return nil, fmt.Errorf("error parsing package data: %v", err)
	}
	return packageDetails, nil
}
