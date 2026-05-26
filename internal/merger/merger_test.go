package merger

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// A trimmed-down but format-faithful RINEX 2.11 observation file. The
// header has just enough labels to be recognisable; the data section is
// one epoch with one satellite worth of observation values. Tests use
// two of these to verify that the merge keeps one header and stacks both
// data sections in order.
const fileA = `     2.11           OBSERVATION DATA    M (MIXED)           RINEX VERSION / TYPE
test program        test agency         20210725 23:00:00UTCPGM / RUN BY / DATE
NYBP                                                        MARKER NAME
 2021     7    25    23     0    0.0000000     GPS         TIME OF FIRST OBS
                                                            END OF HEADER
 21  7 25 23  0  0.0000000  0  1G01
  111111111.111 6  88888888.444 6  20000000.0  20000000.0  40.0  35.0
`

const fileB = `     2.11           OBSERVATION DATA    M (MIXED)           RINEX VERSION / TYPE
test program        test agency         20210726 00:00:00UTCPGM / RUN BY / DATE
NYBP                                                        MARKER NAME
 2021     7    26     0     0    0.0000000     GPS         TIME OF FIRST OBS
                                                            END OF HEADER
 21  7 26  0  0  0.0000000  0  1G01
  222222222.222 7  99999999.555 7  20000111.1  20000111.1  41.0  36.0
`

func readers(s ...string) []io.Reader {
	out := make([]io.Reader, len(s))
	for i, v := range s {
		out[i] = strings.NewReader(v)
	}
	return out
}

func TestMergeKeepsOneHeaderAndAllData(t *testing.T) {
	var out bytes.Buffer
	n, err := Merge(&out, readers(fileA, fileB))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 sources merged, got %d", n)
	}
	got := out.String()

	if c := strings.Count(got, "RINEX VERSION / TYPE"); c != 1 {
		t.Errorf("expected 1 VERSION line, got %d", c)
	}
	if c := strings.Count(got, "END OF HEADER"); c != 1 {
		t.Errorf("expected 1 END OF HEADER, got %d", c)
	}
	idxA := strings.Index(got, "111111111.111")
	idxB := strings.Index(got, "222222222.222")
	if idxA < 0 || idxB < 0 || idxA >= idxB {
		t.Errorf("data ordering off: idxA=%d idxB=%d", idxA, idxB)
	}
	if c := strings.Count(got, " 21  7 2"); c != 2 {
		t.Errorf("expected 2 epoch headers, got %d", c)
	}
	commentIdx := strings.Index(got, "merged 2 hourly RINEX blocks")
	eohIdx := strings.Index(got, "END OF HEADER")
	if commentIdx < 0 || commentIdx > eohIdx {
		t.Errorf("merge comment should land in the header; commentIdx=%d eohIdx=%d", commentIdx, eohIdx)
	}
}

func TestMergeSingleSourceJustWritesItOut(t *testing.T) {
	var out bytes.Buffer
	n, err := Merge(&out, readers(fileA))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 source, got %d", n)
	}
	if !strings.Contains(out.String(), "merged 1 hourly RINEX blocks") {
		t.Error("expected merge comment to still appear for 1-source merge")
	}
	if !strings.Contains(out.String(), "111111111.111") {
		t.Error("data line missing")
	}
}

func TestMergeRefusesEmptyInput(t *testing.T) {
	_, err := Merge(new(bytes.Buffer), nil)
	if err == nil {
		t.Error("expected error on zero sources")
	}
}
