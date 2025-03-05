package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

const NPM_REGISTRY = "https://registry.npmjs.org"

var client = resty.NewWithClient(&http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConnsPerHost: 2,
	},
}).
	SetRetryCount(5).
	SetRetryMaxWaitTime(2 * time.Minute).
	AddRetryCondition(
		func(r *resty.Response, err error) bool {
			return r.StatusCode() == http.StatusTooManyRequests ||
				r.StatusCode() >= http.StatusInternalServerError
		},
	).
	SetRetryAfter(func(client *resty.Client, resp *resty.Response) (time.Duration, error) {
		if retryAfter := resp.Header().Get("Retry-After"); retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil {
				return time.Duration(seconds) * time.Second, nil
			}
			if retryTime, err := http.ParseTime(retryAfter); err == nil {
				return time.Until(retryTime), nil
			}
		}

		attempt := resp.Request.Attempt
		base := time.Duration(1<<uint(attempt)) * time.Second
		jitter := time.Duration(rand.Int63n(int64(base)))
		return base + jitter, nil
	})

func NpmRegistry(packageName *string) (PackageData, error) {
	url := fmt.Sprintf("%s/%s/latest", NPM_REGISTRY, *packageName)
	respBody, err := NpmGetBytes(url)
	if err != nil {
		return PackageData{}, fmt.Errorf("failed to fetch package: %w", err)
	}
	defer respBody.Close()

	var packageData PackageData
	if err := json.NewDecoder(respBody).Decode(&packageData); err != nil {
		return PackageData{}, fmt.Errorf("decode error: %w", err)
	}
	return packageData, nil
}

func NpmRegistryVersionData(packageName *string) (VersionsData, error) {
	url := fmt.Sprintf("%s/%s", NPM_REGISTRY, *packageName)
	respBody, err := NpmGetBytes(url)
	if err != nil {
		return VersionsData{}, fmt.Errorf("failed to fetch versions: %w", err)
	}
	defer respBody.Close()

	var versionsData VersionsData
	if err := json.NewDecoder(respBody).Decode(&versionsData); err != nil {
		return VersionsData{}, fmt.Errorf("decode error: %w", err)
	}
	return versionsData, nil
}

func NpmGetBytes(route string) (io.ReadCloser, error) {
	resp, err := client.R().
		SetHeader("Accept", "application/vnd.npm.install-v1+json; q=1.0, application/json; q=0.8, */*").
		SetDoNotParseResponse(true).
		Get(route)

	if err != nil {
		return nil, fmt.Errorf("request failed after %d attempts: %w",
			resp.Request.Attempt, err)
	}

	if resp.StatusCode() != http.StatusOK {
		retryAfter := resp.Header().Get("Retry-After")
		body, _ := io.ReadAll(resp.RawResponse.Body)
		resp.RawResponse.Body.Close()

		return nil, fmt.Errorf("status %d for %s (Retry-After: %s, Body: %s)",
			resp.StatusCode(),
			route,
			retryAfter,
			string(body),
		)
	}

	return resp.RawResponse.Body, nil
}
