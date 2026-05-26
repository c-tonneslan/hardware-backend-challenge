package timewindow

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestBlockFilenameMatchesNOAAConvention(t *testing.T) {
	cases := []struct {
		name  string
		block Block
		want  string
	}{
		{"hour 1 day 207", Block{"nybp", 2021, 207, 1}, "nybp207b.21o.gz"},
		{"hour 23 day 206", Block{"nybp", 2021, 206, 23}, "nybp206x.21o.gz"},
		{"hour 0 day 1 of year 2000", Block{"abcd", 2000, 1, 0}, "abcd001a.00o.gz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.block.Filename(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHoursMatchesExampleFromBrief(t *testing.T) {
	// From CHALLENGE: `./grab_data nybp 2021-07-25T23:11:22Z 2021-07-26T01:33:44Z`
	// should pull nybp206x, nybp207a, nybp207b.
	blocks, err := Hours("nybp",
		mustParse(t, "2021-07-25T23:11:22Z"),
		mustParse(t, "2021-07-26T01:33:44Z"))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(blocks))
	for i, b := range blocks {
		got[i] = b.Filename()
	}
	want := []string{"nybp206x.21o.gz", "nybp207a.21o.gz", "nybp207b.21o.gz"}
	if len(got) != len(want) {
		t.Fatalf("got %d blocks (%v), want 3 (%v)", len(got), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("block %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHoursExclusiveOnEndExactBoundary(t *testing.T) {
	// An end of exactly 01:00 should NOT include the 01:00 block.
	blocks, err := Hours("nybp",
		mustParse(t, "2021-07-25T23:11:22Z"),
		mustParse(t, "2021-07-26T01:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Errorf("expected 2 blocks (23 and 00), got %d: %v", len(blocks), blocks)
	}
}

func TestHoursRejectsBadInputs(t *testing.T) {
	if _, err := Hours("", mustParse(t, "2021-07-25T23:00:00Z"), mustParse(t, "2021-07-26T01:00:00Z")); err == nil {
		t.Error("expected error on empty station")
	}
	if _, err := Hours("nybp", mustParse(t, "2021-07-26T01:00:00Z"), mustParse(t, "2021-07-25T23:00:00Z")); err == nil {
		t.Error("expected error on end <= start")
	}
}

func TestHoursCrossesDayOfYearBoundary(t *testing.T) {
	blocks, err := Hours("nybp",
		mustParse(t, "2021-12-31T23:00:00Z"),
		mustParse(t, "2022-01-01T01:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks across year boundary, got %d", len(blocks))
	}
	if blocks[0].Year != 2021 || blocks[0].DOY != 365 || blocks[0].Hour != 23 {
		t.Errorf("first block: %+v", blocks[0])
	}
	if blocks[1].Year != 2022 || blocks[1].DOY != 1 || blocks[1].Hour != 0 {
		t.Errorf("second block: %+v", blocks[1])
	}
}

func TestBlockPath(t *testing.T) {
	b := Block{"nybp", 2021, 207, 1}
	want := "/corsdata/rinex/2021/207/nybp/nybp207b.21o.gz"
	if got := b.Path(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
