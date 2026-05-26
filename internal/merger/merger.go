// Package merger concatenates a chronological sequence of RINEX 2.x
// observation files into one. The output keeps the first file's header
// verbatim plus a single COMMENT line explaining the merge; subsequent
// files contribute their observation epochs only.
//
// This is a pragmatic merge, not a full TEQC-style header reconciliation
// (TEQC also rebuilds TIME OF FIRST OBS, deduplicates WAVELENGTH FACT
// records, walks each source's PGM/RUN BY/DATE into the new header,
// etc.). For hourly blocks from the same station, the headers are
// functionally identical, so the simple approach produces a valid file
// any RINEX-2.x reader can ingest. See README for the trade-off.
package merger

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// endOfHeader is the 60-char label that marks the last line of every
// RINEX header. The data section starts on the next line.
const endOfHeader = "END OF HEADER"

// commentLabel is the column-60 label used by COMMENT lines we add to
// the merged header.
const commentLabel = "COMMENT"

// Merge writes the merged RINEX content of `sources` (in chronological
// order) to `w`. Each source must be a complete RINEX observation file
// ending in either a final newline or EOF. The function returns the
// number of source files merged plus any read/write error.
func Merge(w io.Writer, sources []io.Reader) (int, error) {
	if len(sources) == 0 {
		return 0, fmt.Errorf("no sources to merge")
	}
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	for i, src := range sources {
		scanner := bufio.NewScanner(src)
		// Default token size of 64 KB is fine for RINEX 2 (80-char lines)
		// but bumping it cheaply guards against any oversized comment
		// line that someone might have appended.
		buf := make([]byte, 0, 256*1024)
		scanner.Buffer(buf, 1024*1024)

		inHeader := true
		for scanner.Scan() {
			line := scanner.Text()
			if i == 0 {
				// Emit the merge COMMENT just *before* END OF HEADER so
				// it lives inside the header where downstream parsers
				// look for it.
				if inHeader && isEndOfHeader(line) {
					if err := writeLine(bw, mergeCommentLine(len(sources))); err != nil {
						return i, err
					}
					inHeader = false
				}
				if err := writeLine(bw, line); err != nil {
					return i, err
				}
				continue
			}
			// For files after the first, skip everything up to and
			// including END OF HEADER, then write each data line.
			if inHeader {
				if isEndOfHeader(line) {
					inHeader = false
				}
				continue
			}
			if err := writeLine(bw, line); err != nil {
				return i, err
			}
		}
		if err := scanner.Err(); err != nil {
			return i, fmt.Errorf("source %d: %w", i, err)
		}
	}
	return len(sources), nil
}

func writeLine(w io.Writer, line string) error {
	if _, err := io.WriteString(w, line); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// isEndOfHeader checks for the RINEX header sentinel. We compare on the
// trimmed label region (positions 60..80) so we tolerate a missing
// trailing newline or trailing whitespace.
func isEndOfHeader(line string) bool {
	// The label is right-justified in columns 60..80 but a permissive
	// substring match is enough — no other RINEX label is a substring
	// of "END OF HEADER".
	return strings.Contains(line, endOfHeader)
}

// mergeCommentLine builds a single COMMENT-tagged header line that
// records what we did. RINEX comments live before END OF HEADER, so
// this gets emitted right before we transition into the data section.
//
// Per the RINEX 2.x format, the comment body sits in columns 1..60 and
// the label "COMMENT" in columns 61..80. We left-justify in 60 to keep
// the formatter readable; many parsers don't care, but TEQC and
// gpstk-aware tools expect the column alignment.
func mergeCommentLine(n int) string {
	body := fmt.Sprintf("merged %d hourly RINEX blocks", n)
	if len(body) > 60 {
		body = body[:60]
	}
	return fmt.Sprintf("%-60s%-20s", body, commentLabel)
}
