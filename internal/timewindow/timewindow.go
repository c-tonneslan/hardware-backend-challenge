// Package timewindow enumerates the NOAA CORS hourly RINEX blocks that
// cover an arbitrary [start, end) time range.
//
// NOAA file paths look like:
//
//	rinex/<year>/<doy>/<station>/<station><doy><hour_letter>.<yy>o.gz
//
// The hour letter maps GPS hour-of-day to a single character: 0→a, 1→b,
// …, 23→x. Day-of-year is 1-indexed (Jan 1 = 001). The year suffix in
// the extension is the last two digits of the GPS year.
package timewindow

import (
	"fmt"
	"time"
)

// Block names one hourly RINEX file by its components and assembles the
// canonical filename and remote path.
type Block struct {
	Station string
	Year    int
	DOY     int // day of year, 1-indexed
	Hour    int // 0-23
}

// Filename returns the canonical "<station><doy><h>.<yy>o.gz" name.
func (b Block) Filename() string {
	letter := byte('a') + byte(b.Hour)
	return fmt.Sprintf("%s%03d%c.%02do.gz", b.Station, b.DOY, letter, b.Year%100)
}

// Path is the full URL path under https://geodesy.noaa.gov/corsdata.
func (b Block) Path() string {
	return fmt.Sprintf("/corsdata/rinex/%d/%03d/%s/%s", b.Year, b.DOY, b.Station, b.Filename())
}

// Hours returns every hourly Block needed to cover [start, end). The range
// is truncated to the hour on both sides: a request for 23:11–01:33 spans
// hours 23, 00, 01 in order. Both timestamps must be in UTC; the caller
// is expected to have already converted from anything else.
//
// An end-of-day rollover crosses the day-of-year boundary; an
// end-of-year rollover crosses the year. The walk is one hour at a time
// via time.Time arithmetic, so leap days and DST quirks land correctly.
func Hours(station string, start, end time.Time) ([]Block, error) {
	if !end.After(start) {
		return nil, fmt.Errorf("end (%s) must be after start (%s)", end, start)
	}
	if station == "" {
		return nil, fmt.Errorf("station is required")
	}
	start = start.UTC().Truncate(time.Hour)
	// We want the hour that *contains* the end timestamp, so subtract one
	// nanosecond before truncating: an end of exactly 01:00 should not
	// pull in the 01:00–02:00 block.
	endHour := end.UTC().Add(-time.Nanosecond).Truncate(time.Hour)

	var out []Block
	for t := start; !t.After(endHour); t = t.Add(time.Hour) {
		out = append(out, Block{
			Station: station,
			Year:    t.Year(),
			DOY:     t.YearDay(),
			Hour:    t.Hour(),
		})
	}
	return out, nil
}
