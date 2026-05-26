# Propeller Hardware Backend Challenge — Solution

## Solution by Charlie Tonneslan

This is a public completion of [Propeller's hardware-backend-challenge](https://github.com/PropellerAero/hardware-backend-challenge), built as a portfolio piece. It is not an active application; I'm not currently in Propeller's hiring pipeline. The implementation lives on `master`; the original brief is preserved at the bottom of this README.

**Stack:** Go 1.22, standard library only. No third-party packages.

**Run it:**
```sh
go run ./cmd/grab_data <station> <start_iso8601> <end_iso8601>
# e.g.
go run ./cmd/grab_data nybp 2026-05-24T22:00:00Z 2026-05-25T01:00:00Z
# writes nybp.obs in the current directory
```

**Run the tests:**
```sh
go test ./...
# 14 cases across timewindow, fetcher, and merger.
```

### What's implemented

A single CLI binary, `grab_data`, that:

1. Takes a base station ID + an ISO 8601 start and end time.
2. Enumerates every hourly NOAA CORS RINEX block whose `[hour, hour+1)` window intersects the requested range.
3. Downloads each block in parallel (default 4 concurrent), decompresses the gzip envelope, and reads it fully into memory.
4. Merges the per-hour bodies into one RINEX 2.x observation file, keeping the first source's header and concatenating each subsequent file's data section after its own END OF HEADER.
5. Writes the merged output to `<station>.obs` (override with `--out path`).

Verified end-to-end against the real NOAA server: a 3-hour pull on `nybp` produces a ~44 MB merged file with one header, three concatenated data sections, and a `merged N hourly RINEX blocks` COMMENT recorded inside the header.

### Architecture

```
cmd/grab_data/                CLI entry point: flag parsing, parallel fetch, error mapping
internal/
  timewindow/                 enumerate hourly blocks across [start, end); URL/filename math
  fetcher/                    HTTP GET + gzip decompression; ErrNotFound surfaces 404 separately
  merger/                     concat RINEX 2.x bodies, single header + a merge COMMENT
```

Each internal package has its own tests using only the standard library. Fetcher tests stand up an `httptest.Server` so the HTTP path is exercised without hitting the real NOAA endpoint.

### Design decisions

- **Standard library only.** No `cobra`, no `gorilla`, no third-party gzip. The whole thing is `flag`, `net/http`, `compress/gzip`, `bufio`, `time`. The brief calls out "Feel free to use any other libraries or toolkits you need," but a small CLI is exactly where the stdlib shines.

- **Parallel fetch with ordered slots.** Downloads run concurrently bounded by `--concurrency`, but the bodies land in a slice indexed by the chronological position of their block, so the merger sees them in order regardless of which one finishes first. The merge step itself is single-threaded.

- **Pragmatic merge, not a TEQC reimplementation.** TEQC rebuilds `TIME OF FIRST OBS` and merges the `PGM / RUN BY / DATE`, `COMMENT`, and observation-type metadata from every source into one comprehensive header. For hourly blocks from the *same station* the headers are functionally identical, so I keep the first file's header verbatim, add a single `merged N hourly RINEX blocks` COMMENT just before END OF HEADER, and concatenate each subsequent file's data section. Output passes the round-trip check (one VERSION line, one END OF HEADER, N epoch headers). The README notes this is not a drop-in TEQC replacement; for archival reproducibility, callers should still run TEQC. See "What I'd do with more time."

- **Missing-file warnings, not hard failures.** The brief mentions "only a few days worth of the hourly logs are kept on the server." If a single hourly block returns 404, `grab_data` logs a warning (with a hint about the unimplemented daily-file fallback) and keeps going. Only an empty set of successful downloads is a hard failure.

- **Context-aware shutdown.** Ctrl-C cancels the in-flight HTTP calls and propagates through the WaitGroup; the output file is left untouched.

- **Station ID lower-cased.** NOAA's paths are lowercase; accepting any casing from the CLI but lower-casing before building the URL avoids a surprise 404.

### Tests worth calling out

- `TestHoursMatchesExampleFromBrief` — the exact request from the brief (`nybp 2021-07-25T23:11:22Z 2021-07-26T01:33:44Z`) produces `nybp206x`, `nybp207a`, `nybp207b` in that order.
- `TestHoursExclusiveOnEndExactBoundary` — an end of exactly `01:00:00` includes hour 0 but not hour 1.
- `TestHoursCrossesDayOfYearBoundary` — a year-end window correctly walks DOY 365 → DOY 1 with the year bump.
- `TestMergeKeepsOneHeaderAndAllData` — confirms the merge produces exactly one VERSION line, one END OF HEADER, and stacks both data sections in chronological order with the merge COMMENT inside the header.
- `TestGet404IsErrNotFound` and `TestGet5xxIsRegularError` — pin down the failure semantics the CLI relies on.

### What I'd do with more time

- **Daily-file fallback.** The stretch goal in the brief: if `<station><doy><h>.<yy>o.gz` returns 404, retry against `<station><doy>0.<yy>o.gz` (full-day file). The fetcher already surfaces a distinct `ErrNotFound` so the CLI can branch on it cleanly; the daily-file URL is one extra path-builder method and a single retry call.
- **TEQC-fidelity header merging.** Walk every source's header, rebuild `TIME OF FIRST OBS` from the first observation epoch, merge `WAVELENGTH FACT` records, and write a comprehensive header instead of borrowing file 1's verbatim. The current output is correct for typical hourly merges from a single station; multi-day or multi-station merges would benefit.
- **Streaming merge.** The body of each block is currently read into memory before merge. NOAA hourly blocks are ~50-200 KB so the whole day stays under 5 MB, but a streaming pipeline (decompress → merge directly) would scale to month-long pulls.
- **Retry on transient failures.** Today a transient network error or 502 fails the whole run. A short exponential backoff on 5xx and connection errors would ride out NOAA's occasional hiccups.
- **An integration test that hits the real NOAA server.** Behind `go test -tags=integration`, so CI can opt in. Currently I verified the end-to-end manually (and the output matches the reference `example.obs` shape).

### Smoke test

```sh
go build -o grab_data ./cmd/grab_data
./grab_data --out /tmp/nybp.obs nybp 2026-05-24T22:00:00Z 2026-05-25T01:00:00Z
# Result on my machine: 3 blocks downloaded, 44 MB output, 1 header, 3 epoch streams
```

---

## Original brief

(Preserved verbatim from upstream.)

### Background

Centimetre accurate GNSS positions are generally obtained using differential carrier-phase techniques. These methods use a pair of GNSS receivers, one called the "base" which stays at a fixed, known location and the other known as the "rover" which is placed at unknown locations which are to be measured.

By combining satellite signal and carrier wave observation data from both receivers, variations in the satellite orbit and atmospheric conditions which lead to error in the position solution can be cancelled out.

Networks of fixed GNSS base stations known as continuously operating reference station (CORS) networks are operated by government departments and private companies. These networks broadcast the base station observation data as well as their well-known position in real time over the internet for use in surveying and machine control applications, most also make archives of the data available for post-processing.

There is a standardised open-source ASCII file format called RINEX (Receiver Independent Exchange Format) which these observations are made available in.

One of these networks in the NOAA CORS network in the United States. RINEX observation data from each of their base stations is published in 1 hour blocks on their HTTP server at https://geodesy.noaa.gov/corsdata/rinex.

### The Task

Write a command line tool in a language of your choice that given a base station ID, start time and end time downloads all the 1 hour blocks that span the time period given and merges them into a single RINEX observation file called `<base_station_id>.obs`. The start and end times will be given as ISO8601 strings.

The RINEX observation files are stored in gzip'd archives on the server at paths similar to this example:

https://geodesy.noaa.gov/corsdata/rinex/2021/206/nybp/nybp206b.21o.gz

Where:

- `2021` is the year
- `206` is the day of the year
- `nybp` is the base station ID
- `b` refers to the hour block. `a` refers to the hour from 12am-1am (GPS time), `x` refers to 11pm-12am, and so on
- The `"21"` in the `.21o` extension is the year again. Files from 2012 will end with `.22o`

### Example

`./grab_data nybp 2021-07-25T23:11:22Z 2021-07-26T01:33:44Z`

Should download the following files from the server and generate the example output `example.obs` contained in this repository:

    https://geodesy.noaa.gov/corsdata/rinex/2021/206/nybp/nybp206x.21o.gz
    https://geodesy.noaa.gov/corsdata/rinex/2021/207/nybp/nybp207a.21o.gz
    https://geodesy.noaa.gov/corsdata/rinex/2021/207/nybp/nybp207b.21o.gz

### Things looking for

- Ease of installation and use
- Robust error handling
- Well tested code, including some unit tests
- Appropriately commented code

### Helpful resources

- The [TEQC toolkit](https://www.unavco.org/software/data-processing/teqc/teqc.html) is a CLI tool that can merge RINEX files
- There is a useful GPS calendar [here](https://www.gnsscalendar.com/)
- The RINEX format specification can be found [here](http://www.geophys.ac.cn/doc/GNSS/rinexv210.pdf)
