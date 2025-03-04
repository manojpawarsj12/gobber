package internal

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
		return PackageData{}, err
	}
	defer respBody.Close()

	var packageData PackageData
	err = json.NewDecoder(respBody).Decode(&packageData)
	if err != nil {
		return PackageData{}, fmt.Errorf("error decoding package data: %v", err)
	}

	return packageData, nil
}

func NpmRegistryVersionData(packageName *string) (VersionsData, error) {
	respBody, err := NpmGetBytes(fmt.Sprintf("%s/%s", NPM_REGISTRY, *packageName))
	if err != nil {
		return VersionsData{}, err
	}
	defer respBody.Close()

	var versionsData VersionsData
	err = json.NewDecoder(respBody).Decode(&versionsData)
	if err != nil {
		return VersionsData{}, fmt.Errorf("error decoding versions data: %v", err)
	}

	return versionsData, nil
}

func NpmGetBytes(route string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, route, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Add("Accept", "application/vnd.npm.install-v1+json; q=1.0, application/json; q=0.8, */*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return resp.Body, nil
}
