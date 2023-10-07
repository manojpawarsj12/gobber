package gobber

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
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
		log.Println("Error in NpmRegistry: %v", err)
		return PackageData{}, err
	}

	var packageData PackageData
	err = json.NewDecoder(respBody).Decode(&packageData)
	if err != nil {
		log.Println("Error decoding JSON in NpmRegistry: %v", err.Error())
		return PackageData{}, err
	}

	return packageData, nil
}

func NpmRegistryVersionData(packageName *string) (VersionsData, error) {
	respBody, err := NpmGetBytes(fmt.Sprintf("%s/%s", NPM_REGISTRY, *packageName))
	if err != nil {
		log.Printf("Error in NpmRegistryVersionData: %v", err)
		return VersionsData{}, err
	}

	var versionsData VersionsData
	err = json.NewDecoder(respBody).Decode(&versionsData)
	if err != nil {
		log.Printf("Error decoding JSON in NpmRegistryVersionData: %v", err)
		return VersionsData{}, err
	}

	return versionsData, nil
}

func NpmGetBytes(route string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, route, nil)
	if err != nil {
		log.Println("Error creating HTTP request in NpmGetBytes: %v", err)
		return nil, err
	}
	req.Header.Add("Accept", "application/vnd.npm.install-v1+json; q=1.0, application/json; q=0.8, */*")
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error making HTTP request in NpmGetBytes: %v", err)
		return nil, err
	}
	log.Printf("Response code for route %s is %d\n", route, resp.StatusCode)
	return resp.Body, nil
}
