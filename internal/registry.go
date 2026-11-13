package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const DefaultRegistry = "https://registry.npmjs.org"

// Registry is an npm registry client that caps concurrent requests.
type Registry struct {
	URL    string
	client *http.Client
	sem    chan struct{}
}

func NewRegistry(url string, concurrency int) *Registry {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConnsPerHost = concurrency
	return &Registry{
		URL:    strings.TrimSuffix(url, "/"),
		client: &http.Client{Transport: t, Timeout: 5 * time.Minute},
		sem:    make(chan struct{}, concurrency),
	}
}

// Packument fetches abbreviated metadata ("corgi" format, much smaller than the full document).
func (r *Registry) Packument(name string) (*Packument, error) {
	body, err := r.get(r.URL+"/"+strings.Replace(name, "/", "%2f", 1),
		"application/vnd.npm.install-v1+json; q=1.0, application/json; q=0.8")
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var p Packument
	if err := json.NewDecoder(body).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode %s metadata: %w", name, err)
	}
	return &p, nil
}

// get holds a concurrency slot until the returned body is closed.
// shortcut: ignores Retry-After, uses fixed backoff; honour it if npm starts throttling us.
func (r *Registry) get(url, accept string) (io.ReadCloser, error) {
	r.sem <- struct{}{}
	release := func() { <-r.sem }
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*attempt) * 250 * time.Millisecond)
		}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			release()
			return nil, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		resp, err := r.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return &slotBody{ReadCloser: resp.Body, release: release}, nil
		}
		resp.Body.Close()
		lastErr = fmt.Errorf("GET %s: %s", url, resp.Status)
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			break
		}
	}
	release()
	return nil, lastErr
}

type slotBody struct {
	io.ReadCloser
	release func()
	once    sync.Once
}

func (b *slotBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
}
