package fetcher

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c-tonneslan/hardware-backend-challenge/internal/timewindow"
)

func gzipBytes(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGetDecompressesGzipResponse(t *testing.T) {
	want := "    RINEX VERSION / TYPE\nEND OF HEADER\n21 1 1 0 0 0\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "nybp206x.21o.gz") {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/x-gzip")
		_, _ = w.Write(gzipBytes(t, want))
	}))
	defer srv.Close()

	f := &Fetcher{BaseURL: srv.URL, Client: srv.Client()}
	got, err := f.Get(context.Background(), timewindow.Block{
		Station: "nybp", Year: 2021, DOY: 206, Hour: 23,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGet404IsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer srv.Close()

	f := &Fetcher{BaseURL: srv.URL, Client: srv.Client()}
	_, err := f.Get(context.Background(), timewindow.Block{
		Station: "nybp", Year: 2021, DOY: 206, Hour: 23,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGet5xxIsRegularError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Server Down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := &Fetcher{BaseURL: srv.URL, Client: srv.Client()}
	_, err := f.Get(context.Background(), timewindow.Block{
		Station: "nybp", Year: 2021, DOY: 206, Hour: 23,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("a 500 should not be ErrNotFound, got %v", err)
	}
}

func TestGetReportsBadGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not gzip data"))
	}))
	defer srv.Close()

	f := &Fetcher{BaseURL: srv.URL, Client: srv.Client()}
	_, err := f.Get(context.Background(), timewindow.Block{
		Station: "nybp", Year: 2021, DOY: 206, Hour: 23,
	})
	if err == nil {
		t.Fatal("expected gzip error")
	}
}
