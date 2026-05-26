// Package fetcher pulls a single gzipped RINEX block off the NOAA CORS
// HTTP server and returns its decompressed bytes.
package fetcher

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/c-tonneslan/hardware-backend-challenge/internal/timewindow"
)

// ErrNotFound is returned when the server reports a 404 for the requested
// block. The CLI uses it to drive the "fall back to daily file" stretch
// goal — only a missing-file response should trigger the fallback, not
// any other class of error.
var ErrNotFound = errors.New("block not found on server")

// DefaultBaseURL is NOAA's CORS RINEX root.
const DefaultBaseURL = "https://geodesy.noaa.gov"

// Fetcher knows how to retrieve and decompress hourly RINEX blocks.
// BaseURL and Client are exported so a test can point at an httptest
// server with a custom round-tripper.
type Fetcher struct {
	BaseURL string
	Client  *http.Client
}

func New() *Fetcher {
	return &Fetcher{
		BaseURL: DefaultBaseURL,
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Get fetches one block and returns its decompressed RINEX text. The
// underlying response body is read fully into memory; an hour of CORS
// observation data is ~50-200 KB uncompressed, so this stays well within
// any reasonable budget even for a day's worth of blocks.
func (f *Fetcher) Get(ctx context.Context, b timewindow.Block) ([]byte, error) {
	url := f.BaseURL + b.Path()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// NOAA's frontend is more cooperative when callers identify
	// themselves; setting a UA stops them rate-limiting "go-http-client".
	req.Header.Set("User-Agent", "hardware-backend-challenge/1.0")

	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%w: %s", ErrNotFound, url)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		// Drain a bit of the body for the error message; never the
		// whole thing because a 500 page can be large.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("get %s: status %d: %s", url, resp.StatusCode, body)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gunzip %s: %w", url, err)
	}
	defer gz.Close()

	out, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	return out, nil
}
