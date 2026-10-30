package gnudate

import (
	"testing"
	"time"
)

// base is a Friday morning, so the weekday cases have stable expectations.
var base = time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want time.Time
	}{
		// Keywords.
		{"now", "now", base},
		{"today", "today", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"tomorrow", "tomorrow", time.Date(2024, 3, 16, 0, 0, 0, 0, time.UTC)},
		{"yesterday", "yesterday", time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)},
		{"midnight", "midnight", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"noon", "noon", time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)},

		// Epoch times.
		{"epoch", "@1700000000", time.Unix(1700000000, 0)},
		{"negative epoch fraction", "@-0.5", time.Unix(0, -500000000)},
		{"the empty string is the beginning of today", "", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"blank is the empty string", "  ", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"epoch fraction", "@1700000000.5", time.Unix(1700000000, 500000000)},
		{"negative epoch", "@-1", time.Unix(-1, 0)},

		// ISO dates and times.
		{"iso date", "2020-01-02", time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)},
		{"iso datetime", "2020-01-02T03:04", time.Date(2020, 1, 2, 3, 4, 0, 0, time.UTC)},
		{"iso datetime seconds", "2020-01-02T03:04:05", time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)},
		{"date and time", "2020-01-02 03:04:05", time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)},
		{"datetime with zone", "2020-01-02T03:04:05+0530", time.Date(2020, 1, 1, 21, 34, 5, 0, time.UTC)},
		{"time", "03:04", time.Date(2024, 3, 15, 3, 4, 0, 0, time.UTC)},
		{"time seconds", "14:19:13", time.Date(2024, 3, 15, 14, 19, 13, 0, time.UTC)},
		{"time fraction", "14:19:13.25", time.Date(2024, 3, 15, 14, 19, 13, 250000000, time.UTC)},
		{"time with zone", "14:19:13+0530", time.Date(2024, 3, 15, 8, 49, 13, 0, time.UTC)},
		{"time 12 hour", "2:30pm", time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)},

		// Month-name dates.
		{"month day", "Jul 21 14:19", time.Date(2024, 7, 21, 14, 19, 0, 0, time.UTC)},
		{"month day time year", "Jul 21 14:19:13 2020", time.Date(2020, 7, 21, 14, 19, 13, 0, time.UTC)},
		{"day month year", "21 Jul 2020", time.Date(2020, 7, 21, 0, 0, 0, 0, time.UTC)},
		{"month day comma year", "July 21, 2020", time.Date(2020, 7, 21, 0, 0, 0, 0, time.UTC)},
		{"year month day", "2020 Jul 21", time.Date(2020, 7, 21, 0, 0, 0, 0, time.UTC)},
		{"month year", "Jul 2020", time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC)},

		// Relative items.
		{"minutes", "5 minutes", base.Add(5 * time.Minute)},
		{"negative minutes", "-5 minutes", base.Add(-5 * time.Minute)},
		{"plus hours", "+2 hours", base.Add(2 * time.Hour)},
		{"days ago", "2 days ago", base.AddDate(0, 0, -2)},
		{"one week", "1 week", base.AddDate(0, 0, 7)},
		{"months", "3 months", base.AddDate(0, 3, 0)},
		{"year", "1 year", base.AddDate(1, 0, 0)},
		{"abbreviated unit", "5 mins", base.Add(5 * time.Minute)},
		{"single letter unit", "2 h", base.Add(2 * time.Hour)},
		{"next week", "next week", base.AddDate(0, 0, 7)},
		{"last month", "last month", base.AddDate(0, -1, 0)},
		{"next hour", "next hour", base.Add(time.Hour)},

		// Weekdays.
		{"coming monday", "monday", time.Date(2024, 3, 18, 10, 30, 0, 0, time.UTC)},
		{"same weekday is today", "friday", base},
		{"next friday", "next friday", time.Date(2024, 3, 22, 10, 30, 0, 0, time.UTC)},
		{"last monday", "last monday", time.Date(2024, 3, 11, 10, 30, 0, 0, time.UTC)},
		{"weekday prefix", "tues", time.Date(2024, 3, 19, 10, 30, 0, 0, time.UTC)},

		// Combinations.
		{"date plus days", "2020-01-01 +2 days", time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC)},
		{"yesterday at time", "yesterday 10:00", time.Date(2024, 3, 14, 10, 0, 0, 0, time.UTC)},
		{"tomorrow noon", "tomorrow noon", time.Date(2024, 3, 16, 12, 0, 0, 0, time.UTC)},
		{"time then date", "10:00 yesterday", time.Date(2024, 3, 14, 10, 0, 0, 0, time.UTC)},
		{"zone before time", "+0530 03:04", time.Date(2024, 3, 14, 21, 34, 0, 0, time.UTC)},
		{"zone after time", "2020-01-02 03:04 +0530", time.Date(2020, 1, 1, 21, 34, 0, 0, time.UTC)},
		{"zone word", "2020-01-02 UTC", time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Parse(test.in, base, time.UTC)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", test.in, err)
			}
			if !got.Equal(test.want) {
				t.Errorf("Parse(%q) = %v, want %v", test.in, got, test.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{
		"foo",
		"5 furlongs",
		"@abc",
		"@",
		"next fort",
		"Jul 32",
		"32 Jul",
		"20201332",
		"minutes",
		// A date which doesn't exist on the calendar.
		"Feb 30 2020",
		"2023 Feb 29",
		// A single sign, not a run of them.
		"++5 minutes",
		"+-5 minutes",
		// A malformed numeric zone.
		"+5:3",
		// An epoch fraction longer than nanoseconds.
		"@1.1234567890",
	} {
		t.Run(in, func(t *testing.T) {
			if _, err := Parse(in, base, time.UTC); err == nil {
				t.Errorf("Parse(%q) error = nil, want an error", in)
			}
		})
	}
}

func TestParseLocation(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("time zone database not available")
	}

	// A zoneless time is interpreted in the given location.
	got, err := Parse("2020-07-21 14:19:13", base, ny)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if want := time.Date(2020, 7, 21, 14, 19, 13, 0, ny); !got.Equal(want) {
		t.Errorf("Parse() = %v, want %v", got, want)
	}

	// An explicit zone overrides the location.
	got, err = Parse("2020-07-21T14:19:13+0530", base, ny)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if want := time.Date(2020, 7, 21, 8, 49, 13, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Parse() = %v, want %v", got, want)
	}
}
