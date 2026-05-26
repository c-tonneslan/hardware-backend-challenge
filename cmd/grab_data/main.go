// Binary grab_data is the CLI the brief asks for: download every hourly
// RINEX block for a given station + ISO 8601 time range and merge them
// into a single <station>.obs file in the current directory.
//
// Usage:
//
//	./grab_data <station> <start> <end> [--out path] [--concurrency N]
//
// Times are ISO 8601 (e.g., 2021-07-25T23:11:22Z). Station IDs are
// lowercased before use because NOAA's paths are lowercase.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/c-tonneslan/hardware-backend-challenge/internal/fetcher"
	"github.com/c-tonneslan/hardware-backend-challenge/internal/merger"
	"github.com/c-tonneslan/hardware-backend-challenge/internal/timewindow"
)

func main() {
	var (
		out         string
		concurrency int
	)
	flag.StringVar(&out, "out", "", "output path (default: <station>.obs in CWD)")
	flag.IntVar(&concurrency, "concurrency", 4, "max concurrent downloads")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: grab_data [flags] <station> <start> <end>")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  Downloads every hourly RINEX block covering [start, end) from NOAA CORS")
		fmt.Fprintln(os.Stderr, "  and merges them into one observation file.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  Times are ISO 8601 (e.g. 2021-07-25T23:11:22Z).")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 3 {
		flag.Usage()
		os.Exit(2)
	}
	station := strings.ToLower(flag.Arg(0))
	start, err := time.Parse(time.RFC3339, flag.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "grab_data: bad start time: %v\n", err)
		os.Exit(2)
	}
	end, err := time.Parse(time.RFC3339, flag.Arg(2))
	if err != nil {
		fmt.Fprintf(os.Stderr, "grab_data: bad end time: %v\n", err)
		os.Exit(2)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if out == "" {
		out = station + ".obs"
	}

	if err := run(ctx, log, station, start, end, out, concurrency); err != nil {
		fmt.Fprintf(os.Stderr, "grab_data: %v\n", err)
		os.Exit(1)
	}
}

func run(
	ctx context.Context,
	log *slog.Logger,
	station string,
	start, end time.Time,
	out string,
	concurrency int,
) error {
	blocks, err := timewindow.Hours(station, start, end)
	if err != nil {
		return err
	}
	log.Info("planning download", "station", station, "blocks", len(blocks), "start", start, "end", end)

	f := fetcher.New()

	// Download all blocks in parallel. Order matters for the merge, so
	// we fan out into a slice indexed by block position.
	bodies := make([][]byte, len(blocks))
	errs := make([]error, len(blocks))
	sem := make(chan struct{}, max(1, concurrency))
	var wg sync.WaitGroup

	for i, b := range blocks {
		wg.Add(1)
		go func(i int, b timewindow.Block) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			log.Info("fetching", "block", b.Filename())
			body, err := f.Get(ctx, b)
			if err != nil {
				errs[i] = err
				return
			}
			bodies[i] = body
		}(i, b)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}

	// Surface any failures. Missing-file errors get a hint about the
	// daily-fallback stretch goal.
	missing := 0
	for i, err := range errs {
		if err == nil {
			continue
		}
		if errors.Is(err, fetcher.ErrNotFound) {
			log.Warn("block missing on server", "block", blocks[i].Filename(),
				"hint", "older hourly files may be rotated off; the brief's daily-fallback stretch isn't implemented in this submission")
			missing++
			continue
		}
		return fmt.Errorf("fetch %s: %w", blocks[i].Filename(), err)
	}

	// Build the source list, dropping missing blocks. The merger
	// tolerates a partial sequence as long as at least one block landed.
	var sources []io.Reader
	for _, body := range bodies {
		if body == nil {
			continue
		}
		sources = append(sources, bytes.NewReader(body))
	}
	if len(sources) == 0 {
		return fmt.Errorf("no blocks downloaded (all %d missing)", len(blocks))
	}

	outPath, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	fh, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer fh.Close()

	merged, err := merger.Merge(fh, sources)
	if err != nil {
		return fmt.Errorf("merge: %w", err)
	}
	log.Info("done", "written", outPath, "merged_blocks", merged, "missing", missing)
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
