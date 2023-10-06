package gobber

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const NPM_REGISTRY = "https://registry.npmjs.org"

var transport = &http.Transport{
	MaxIdleConns:        10,
	IdleConnTimeout:     30 * time.Second,
	MaxIdleConnsPerHost: 2,
}

var client = &http.Client{
	Transport: transport,
}

func NpmRegistry(packageName *string) (PackageData, error) {
	respBody, err := NpmGetBytes(fmt.Sprintf("%s/%s/%s", NPM_REGISTRY, *packageName, "latest"))
	if err != nil {
		return PackageData{}, fmt.Errorf("Error in NpmRegistry: %v", err)
	}

	var packageData PackageData
	err = json.NewDecoder(respBody).Decode(&packageData)
	if err != nil {
		return PackageData{}, fmt.Errorf("Error decoding JSON in NpmRegistry: %v", err)
	}

	return packageData, nil
}

func NpmRegistryVersionData(packageName *string) (VersionsData, error) {
	respBody, err := NpmGetBytes(fmt.Sprintf("%s/%s", NPM_REGISTRY, *packageName))
	if err != nil {
		return VersionsData{}, fmt.Errorf("Error in NpmRegistryVersionData: %v", err)
	}

	var versionsData VersionsData
	err = json.NewDecoder(respBody).Decode(&versionsData)
	if err != nil {
		return VersionsData{}, fmt.Errorf("Error decoding JSON in NpmRegistryVersionData: %v", err)
	}

	return versionsData, nil
}

func NpmGetBytes(route string) (io.ReadCloser, error) {
	fmt.Println(route)
	req, err := http.NewRequest(http.MethodGet, route, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating HTTP request in NpmGetBytes: %v", err)
	}

	req.Header.Add("Accept", "application/vnd.npm.install-v1+json; q=1.0, application/json; q=0.8, */*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error making HTTP request in NpmGetBytes: %v", err)
	}

	fmt.Printf("Response code for package %s is %d\n", route, resp.StatusCode)
	return resp.Body, nil
}
