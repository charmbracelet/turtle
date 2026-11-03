package strftime

import (
	"testing"
	"time"
)

func TestStrftime(t *testing.T) {
	// The time of an example of the specification, in a zone with an offset,
	// plus other times chosen to pin down every conversion: the day before
	// the tenth, the week numbering of the year, and the hours around noon
	// and midnight.
	var (
		june     = time.Date(1990, 6, 26, 9, 58, 10, 0, time.FixedZone("PDT", -7*3600))
		nov      = time.Date(1991, 11, 2, 13, 36, 16, 0, time.UTC)
		pm       = time.Date(1991, 11, 2, 13, 36, 32, 0, time.UTC)
		noon     = time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
		midnight = time.Date(2024, 1, 5, 0, 5, 0, 0, time.UTC)
		sat2024  = time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
		sat2000  = time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
		fri2021  = time.Date(2021, 1, 1, 12, 0, 0, 0, time.UTC)
		mon2024  = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		fri2016  = time.Date(2016, 1, 1, 12, 0, 0, 0, time.UTC)
		frac     = time.Date(2020, 7, 21, 14, 19, 13, 489392193, time.FixedZone("IST", 5*3600+30*60))
		odd      = time.Date(1901, 1, 1, 12, 0, 0, 0, time.FixedZone("LMT", -(4*3600+56*60+2)))
	)

	tests := []struct {
		name   string
		t      time.Time
		format string
		want   string
	}{
		{name: "the default format", t: june, format: "%a %b %e %H:%M:%S %Z %Y", want: "Tue Jun 26 09:58:10 PDT 1990"},
		{name: "an example of the specification", t: nov, format: "DATE: %m/%d/%y%nTIME: %H:%M:%S", want: "DATE: 11/02/91\nTIME: 13:36:16"},
		{name: "the %r example of the specification", t: pm, format: "TIME: %r", want: "TIME: 01:36:32 PM"},

		{name: "%a", t: june, format: "%a", want: "Tue"},
		{name: "%A", t: june, format: "%A", want: "Tuesday"},
		{name: "%b", t: june, format: "%b", want: "Jun"},
		{name: "%h as %b", t: june, format: "%h", want: "Jun"},
		{name: "%B", t: june, format: "%B", want: "June"},
		{name: "%c", t: june, format: "%c", want: "Tue Jun 26 09:58:10 1990"},
		{name: "%C", t: june, format: "%C", want: "19"},
		{name: "%d", t: june, format: "%d", want: "26"},
		{name: "%D", t: june, format: "%D", want: "06/26/90"},
		{name: "%e", t: june, format: "%e", want: "26"},
		{name: "%e with a single-digit day", t: nov, format: "%e", want: " 2"},
		{name: "%F", t: june, format: "%F", want: "1990-06-26"},
		{name: "%g", t: june, format: "%g", want: "90"},
		{name: "%G", t: june, format: "%G", want: "1990"},
		{name: "%H", t: june, format: "%H", want: "09"},
		{name: "%I", t: june, format: "%I", want: "09"},
		{name: "%j", t: june, format: "%j", want: "177"},
		{name: "%m", t: june, format: "%m", want: "06"},
		{name: "%M", t: june, format: "%M", want: "58"},
		{name: "%n", t: june, format: "a%nb", want: "a\nb"},
		{name: "%N", t: frac, format: "%N", want: "489392193"},
		{name: "%N with no nanoseconds", t: june, format: "%N", want: "000000000"},
		{name: "%p", t: june, format: "%p", want: "AM"},
		{name: "%r", t: june, format: "%r", want: "09:58:10 AM"},
		{name: "%R", t: june, format: "%R", want: "09:58"},
		{name: "%S", t: june, format: "%S", want: "10"},
		{name: "%s", t: june, format: "%s", want: "646419490"},
		{name: "%s of a time in another zone", t: frac, format: "%s", want: "1595321353"},
		{name: "%t", t: june, format: "a%tb", want: "a\tb"},
		{name: "%T", t: june, format: "%T", want: "09:58:10"},
		{name: "%u", t: june, format: "%u", want: "2"},
		{name: "%U", t: june, format: "%U", want: "25"},
		{name: "%V", t: june, format: "%V", want: "26"},
		{name: "%w", t: june, format: "%w", want: "2"},
		{name: "%W", t: june, format: "%W", want: "26"},
		{name: "%x", t: june, format: "%x", want: "06/26/90"},
		{name: "%X", t: june, format: "%X", want: "09:58:10"},
		{name: "%y", t: june, format: "%y", want: "90"},
		{name: "%Y", t: june, format: "%Y", want: "1990"},
		{name: "%z", t: june, format: "%z", want: "-0700"},
		{name: "%Z", t: june, format: "%Z", want: "PDT"},
		{name: "%:z", t: june, format: "%:z", want: "-07:00"},
		{name: "%::z", t: june, format: "%::z", want: "-07:00:00"},
		{name: "%:::z drops the zero parts", t: june, format: "%:::z", want: "-07"},
		{name: "%:z with an offset of half an hour", t: frac, format: "%:z", want: "+05:30"},
		{name: "%:::z with an offset of minutes", t: frac, format: "%:::z", want: "+05:30"},
		{name: "%:z with an offset of seconds", t: odd, format: "%:z", want: "-04:56"},
		{name: "%::z with an offset of seconds", t: odd, format: "%::z", want: "-04:56:02"},
		{name: "%:::z keeps the seconds", t: odd, format: "%:::z", want: "-04:56:02"},
		{name: "%:z in UTC", t: nov, format: "%:z", want: "+00:00"},
		{name: "the O modifier before a zone", t: june, format: "%O:z", want: "-07:00"},
		{name: "an unknown conversion with colons is copied as is", t: june, format: "%:q", want: "%:q"},
		{name: "a trailing colon conversion", t: june, format: "%:", want: "%:"},
		{name: "%z in UTC", t: nov, format: "%z", want: "+0000"},
		{name: "%Z in UTC", t: nov, format: "%Z", want: "UTC"},
		{name: "%%", t: june, format: "100%%", want: "100%"},

		{name: "%I at noon", t: noon, format: "%I", want: "12"},
		{name: "%p at noon", t: noon, format: "%p", want: "PM"},
		{name: "%r at noon", t: noon, format: "%r", want: "12:00:00 PM"},
		{name: "%H at midnight", t: midnight, format: "%H", want: "00"},
		{name: "%I at midnight", t: midnight, format: "%I", want: "12"},
		{name: "%p at midnight", t: midnight, format: "%p", want: "AM"},
		{name: "%p in the afternoon", t: pm, format: "%p", want: "PM"},
		{name: "%I in the afternoon", t: pm, format: "%I", want: "01"},

		{name: "the week numbering of a Saturday in June", t: sat2024, format: "%U %W %V %G %g %u %w %j", want: "23 24 24 2024 24 6 6 167"},
		{name: "the week numbering of a Saturday on New Year's Day", t: sat2000, format: "%U %W %V %G %g %u %w", want: "00 00 52 1999 99 6 6"},
		{name: "the week numbering of a Friday on New Year's Day", t: fri2021, format: "%U %W %V %G %g", want: "00 00 53 2020 20"},
		{name: "the week numbering of a Monday on New Year's Day", t: mon2024, format: "%U %W %V %G %g", want: "00 01 01 2024 24"},
		{name: "the week numbering of a late New Year's Day", t: fri2016, format: "%V %G %g", want: "53 2015 15"},

		{name: "the E modifier is ignored", t: nov, format: "%Ey %Ex %Ec", want: "91 11/02/91 Sat Nov  2 13:36:16 1991"},
		{name: "the O modifier is ignored", t: june, format: "%OY %Oy %Od", want: "1990 90 26"},
		{name: "an unknown conversion is copied as is", t: june, format: "%q", want: "%q"},
		{name: "a trailing percent sign", t: june, format: "100%", want: "100%"},
		{name: "a trailing E modifier", t: june, format: "x%E", want: "x%E"},
		{name: "text without conversions", t: june, format: "date and time", want: "date and time"},
		{name: "an empty format", t: june, format: "", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Format(test.format, test.t); got != test.want {
				t.Errorf("Format(%q, %v) = %q, want %q", test.format, test.t, got, test.want)
			}
		})
	}
}
