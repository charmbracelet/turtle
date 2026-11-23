package date

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// clockTime is the time the tests read as the current one, so their output
// doesn't depend on when they run. It is the time of an example of the
// specification.
var clockTime = time.Date(1990, 6, 26, 9, 58, 10, 0, time.UTC)

// refTime is the modification time of the reference files of the tests.
var refTime = time.Date(2020, 3, 1, 12, 30, 0, 0, time.UTC)

type testCase struct {
	name     string
	fsys     fsx.MemoryFS
	env      map[string]string
	stdin    io.Reader
	args     []string
	want     string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	env := test.env
	if env == nil {
		// Without TZ the time zone falls back to the system's local one,
		// which the tests can't rely on.
		env = map[string]string{"TZ": "UTC"}
	}
	rt := &coreutils.Runtime{
		Stdout:  stdout,
		Stderr:  stderr,
		Stdin:   test.stdin,
		Clock:   func() time.Time { return clockTime },
		Environ: func() []string { return testutil.Environ(env) },
	}
	if test.fsys != nil {
		rt.FS = test.fsys
	}
	err := New(rt).Run(t.Context(), test.args...)

	if code := testutil.ExitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.want {
		t.Errorf("Run() stdout = %q, want %q", got, test.want)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
}

func TestRun(t *testing.T) {
	tests := []testCase{
		{
			name: "the current date and time by default",
			want: "Tue Jun 26 09:58:10 UTC 1990\n",
		},
		{
			name: "the current date and time in UTC with -u",
			args: []string{"-u"},
			want: "Tue Jun 26 09:58:10 UTC 1990\n",
		},
		{
			name: "-u with the UTC0 of the specification",
			env:  map[string]string{"TZ": "UTC0"},
			args: []string{"-u"},
			want: "Tue Jun 26 09:58:10 UTC 1990\n",
		},
		{
			name: "the --utc long option",
			args: []string{"--utc"},
			want: "Tue Jun 26 09:58:10 UTC 1990\n",
		},
		{
			name: "the --universal long option",
			args: []string{"--universal"},
			want: "Tue Jun 26 09:58:10 UTC 1990\n",
		},
		{
			name: "a format operand",
			args: []string{"+%Y-%m-%d"},
			want: "1990-06-26\n",
		},
		{
			name: "a format operand with a literal text",
			args: []string{"+year %Y!"},
			want: "year 1990!\n",
		},
		{
			name: "an empty format operand",
			args: []string{"+"},
			want: "\n",
		},
		{
			name: "a format operand with -u",
			args: []string{"-u", "+%H:%M:%S"},
			want: "09:58:10\n",
		},
		{
			name: "a format operand after --",
			args: []string{"--", "+%Y"},
			want: "1990\n",
		},
		{
			name: "the format of the touch -t times",
			args: []string{"-r", "ref.txt", "+%Y%m%d%H%M.%S"},
			fsys: fsx.MemoryFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "202003011230.00\n",
		},
		{
			name: "the modification time of the -r file",
			args: []string{"-r", "ref.txt"},
			fsys: fsx.MemoryFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "Sun Mar  1 12:30:00 UTC 2020\n",
		},
		{
			name: "the -r option with its argument glued",
			args: []string{"-rref.txt"},
			fsys: fsx.MemoryFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "Sun Mar  1 12:30:00 UTC 2020\n",
		},
		{
			name: "the --reference long option",
			args: []string{"--reference", "ref.txt"},
			fsys: fsx.MemoryFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "Sun Mar  1 12:30:00 UTC 2020\n",
		},
		{
			name: "the --reference long option with an attached argument",
			args: []string{"--reference=ref.txt"},
			fsys: fsx.MemoryFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "Sun Mar  1 12:30:00 UTC 2020\n",
		},
		{
			name: "the -r file in a subdirectory",
			args: []string{"-r", "sub/ref.txt"},
			fsys: fsx.MemoryFS{"sub/ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "Sun Mar  1 12:30:00 UTC 2020\n",
		},
		{
			name: "the -r file with a format operand",
			args: []string{"-r", "ref.txt", "+%Y"},
			fsys: fsx.MemoryFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "2020\n",
		},
		{
			name: "the -r file with -u",
			args: []string{"-ur", "ref.txt"},
			fsys: fsx.MemoryFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "Sun Mar  1 12:30:00 UTC 2020\n",
		},
		{
			name:     "the -r option with an empty argument",
			args:     []string{"-r", ""},
			fsys:     fsx.MemoryFS{},
			wantErr:  "date: failed to get timestamp of '': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "the --reference option with an empty attached argument",
			args:     []string{"--reference="},
			fsys:     fsx.MemoryFS{},
			wantErr:  "date: failed to get timestamp of '': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a -r file which does not exist",
			args:     []string{"-r", "missing.txt"},
			fsys:     fsx.MemoryFS{},
			wantErr:  "date: failed to get timestamp of 'missing.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "no file system",
			args:     []string{"-r", "ref.txt"},
			wantErr:  "date: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "the -r option without an argument",
			args:     []string{"-r"},
			wantErr:  "date: option requires an argument -- 'r'\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid option",
			args:     []string{"-x"},
			wantErr:  "date: invalid option -- 'x'\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an unrecognized long option",
			args:     []string{"--bogus"},
			wantErr:  "date: unrecognized option '--bogus'\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an extra operand",
			args:     []string{"+%Y", "extra"},
			wantErr:  "date: extra operand 'extra'\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSetTime(t *testing.T) {
	// The settime operand is parsed but never applied, since the Runtime
	// exposes no way to set the system clock.
	tests := []testCase{
		{
			name:     "the current year is the default",
			args:     []string{"07101230"},
			wantErr:  "date: cannot set date: setting the system clock is not supported\n",
			wantExit: 1,
		},
		{
			name:     "a two-digit year",
			args:     []string{"0710123025"},
			wantErr:  "date: cannot set date: setting the system clock is not supported\n",
			wantExit: 1,
		},
		{
			name:     "a four-digit year",
			args:     []string{"071012302025"},
			wantErr:  "date: cannot set date: setting the system clock is not supported\n",
			wantExit: 1,
		},
		{
			name:     "with -u",
			args:     []string{"-u", "07101230"},
			wantErr:  "date: cannot set date: setting the system clock is not supported\n",
			wantExit: 1,
		},
		{
			name:     "a settime operand of the wrong length",
			args:     []string{"123"},
			wantErr:  "date: invalid date '123'\n",
			wantExit: 1,
		},
		{
			name:     "a settime operand of an odd length",
			args:     []string{"071012301"},
			wantErr:  "date: invalid date '071012301'\n",
			wantExit: 1,
		},
		{
			name:     "a settime operand which is not made of digits",
			args:     []string{"0112hhmm"},
			wantErr:  "date: invalid date '0112hhmm'\n",
			wantExit: 1,
		},
		{
			name:     "a settime operand with an invalid month",
			args:     []string{"13011230"},
			wantErr:  "date: invalid date '13011230'\n",
			wantExit: 1,
		},
		{
			name:     "a settime operand with a month of zero",
			args:     []string{"00011230"},
			wantErr:  "date: invalid date '00011230'\n",
			wantExit: 1,
		},
		{
			name:     "a settime operand with an invalid day",
			args:     []string{"02311230"},
			wantErr:  "date: invalid date '02311230'\n",
			wantExit: 1,
		},
		{
			name:     "a settime operand with an invalid hour",
			args:     []string{"07012530"},
			wantErr:  "date: invalid date '07012530'\n",
			wantExit: 1,
		},
		{
			name:     "a settime operand with an invalid minute",
			args:     []string{"07011260"},
			wantErr:  "date: invalid date '07011260'\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunTimezone(t *testing.T) {
	if _, err := time.LoadLocation("America/New_York"); err != nil {
		t.Skip("time zone database not available")
	}

	t.Run("uses the time zone of TZ", func(t *testing.T) {
		runCase(t, testCase{
			name: "America/New_York",
			env:  map[string]string{"TZ": "America/New_York"},
			want: "Tue Jun 26 05:58:10 EDT 1990\n",
		})
	})

	t.Run("-u overrides TZ", func(t *testing.T) {
		runCase(t, testCase{
			name: "-u with America/New_York",
			env:  map[string]string{"TZ": "America/New_York"},
			args: []string{"-u"},
			want: "Tue Jun 26 09:58:10 UTC 1990\n",
		})
	})

	t.Run("a format operand uses the time zone of TZ", func(t *testing.T) {
		runCase(t, testCase{
			name: "%z in America/New_York",
			env:  map[string]string{"TZ": "America/New_York"},
			args: []string{"+%z %Z"},
			want: "-0400 EDT\n",
		})
	})

	t.Run("an unknown TZ falls back to the local time zone", func(t *testing.T) {
		runCase(t, testCase{
			name: "an invalid TZ",
			env:  map[string]string{"TZ": "bogus"},
			args: []string{"+%Y"},
			want: "1990\n",
		})
	})
}

func TestRunFixedTZ(t *testing.T) {
	// The fixed-offset POSIX forms of TZ need no time zone database.
	tests := []testCase{
		{
			name: "the UTC0 form of the -u option",
			env:  map[string]string{"TZ": "UTC0"},
			want: "Tue Jun 26 09:58:10 UTC 1990\n",
		},
		{
			name: "the GMT0 historical form",
			env:  map[string]string{"TZ": "GMT0"},
			want: "Tue Jun 26 09:58:10 GMT 1990\n",
		},
		{
			name: "a westward offset",
			env:  map[string]string{"TZ": "EST5"},
			want: "Tue Jun 26 04:58:10 EST 1990\n",
		},
		{
			name: "a signed westward offset",
			env:  map[string]string{"TZ": "EST+5"},
			want: "Tue Jun 26 04:58:10 EST 1990\n",
		},
		{
			name: "an eastward offset",
			env:  map[string]string{"TZ": "UTC-5"},
			want: "Tue Jun 26 14:58:10 UTC 1990\n",
		},
		{
			name: "an offset with minutes",
			env:  map[string]string{"TZ": "IST-5:30"},
			want: "Tue Jun 26 15:28:10 IST 1990\n",
		},
		{
			name: "a TZ with DST rules falls back to the local zone",
			env:  map[string]string{"TZ": "EST5EDT"},
			args: []string{"+%Y"},
			want: "1990\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	t.Run("a write error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdout: testutil.ErrorWriter{Err: errors.New("boom")},
			Stderr: new(bytes.Buffer),
			Clock:  func() time.Time { return clockTime },
		}
		err := New(rt).Run(t.Context(), "+%Y")
		if code := testutil.ExitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
	})

	t.Run("a done context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		rt := &coreutils.Runtime{
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
			Clock:  func() time.Time { return clockTime },
		}
		err := New(rt).Run(ctx, "+%Y")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})
}

func TestParseSetTime(t *testing.T) {
	loc := time.FixedZone("TEST", -3*3600)
	tests := []struct {
		name    string
		operand string
		want    time.Time
		wantErr string
	}{
		{
			name:    "the current year is the default",
			operand: "07101230",
			want:    time.Date(1990, 7, 10, 12, 30, 0, 0, loc),
		},
		{
			name:    "a two-digit year in [69,99]",
			operand: "0710123069",
			want:    time.Date(1969, 7, 10, 12, 30, 0, 0, loc),
		},
		{
			name:    "a two-digit year in [00,68]",
			operand: "0710123068",
			want:    time.Date(2068, 7, 10, 12, 30, 0, 0, loc),
		},
		{
			name:    "a four-digit year",
			operand: "071012301968",
			want:    time.Date(1968, 7, 10, 12, 30, 0, 0, loc),
		},
		{
			name:    "a wrong length",
			operand: "0710",
			wantErr: "invalid date '0710'",
		},
		{
			name:    "not only digits",
			operand: "0710123x",
			wantErr: "invalid date '0710123x'",
		},
		{
			name:    "an invalid month",
			operand: "13011230",
			wantErr: "invalid date '13011230'",
		},
		{
			name:    "February 30",
			operand: "02301230",
			wantErr: "invalid date '02301230'",
		},
		{
			name:    "February 29 of a non-leap year",
			operand: "022912301991",
			wantErr: "invalid date '022912301991'",
		},
		{
			name:    "February 29 of a leap year",
			operand: "022912301992",
			want:    time.Date(1992, 2, 29, 12, 30, 0, 0, loc),
		},
		{
			name:    "an invalid hour",
			operand: "07012430",
			wantErr: "invalid date '07012430'",
		},
		{
			name:    "an invalid minute",
			operand: "07011299",
			wantErr: "invalid date '07011299'",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseSetTime(test.operand, clockTime, loc)
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Errorf("parseSetTime(%q) error = %v, want %q", test.operand, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSetTime(%q) error = %v, want nil", test.operand, err)
			}
			if !got.Equal(test.want) {
				t.Errorf("parseSetTime(%q) = %v, want %v", test.operand, got, test.want)
			}
		})
	}
}

func TestParseTZ(t *testing.T) {
	// The want is the fixed clock time formatted in the zone, empty when the
	// value is not a supported fixed-offset form.
	tests := []struct {
		name string
		tz   string
		want string
	}{
		{name: "UTC0", tz: "UTC0", want: "09:58:10 +0000 UTC"},
		{name: "GMT0", tz: "GMT0", want: "09:58:10 +0000 GMT"},
		{name: "a westward offset", tz: "EST5", want: "04:58:10 -0500 EST"},
		{name: "a signed westward offset", tz: "EST+5", want: "04:58:10 -0500 EST"},
		{name: "an eastward offset", tz: "UTC-5", want: "14:58:10 +0500 UTC"},
		{name: "an offset with minutes", tz: "IST-5:30", want: "15:28:10 +0530 IST"},
		{name: "an offset with seconds", tz: "UTC-1:02:03", want: "11:00:13 +0102 UTC"},
		{name: "an hour of 24", tz: "UTC24", want: "09:58:10 -2400 UTC"},
		{name: "a name which is too short", tz: "UT0", want: ""},
		{name: "no offset", tz: "UTC", want: ""},
		{name: "a DST form", tz: "EST5EDT", want: ""},
		{name: "an hour beyond 24", tz: "UTC25", want: ""},
		{name: "a minute beyond 59", tz: "UTC1:60", want: ""},
		{name: "an offset which is not a number", tz: "UTCx", want: ""},
		{name: "a quoted name", tz: "<+05>-5", want: ""},
		{name: "an empty value", tz: "", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loc := parseTZ(test.tz)
			if test.want == "" {
				if loc != nil {
					t.Errorf("parseTZ(%q) = %v, want nil", test.tz, loc)
				}
				return
			}
			if loc == nil {
				t.Fatalf("parseTZ(%q) = nil, want a zone", test.tz)
			}
			if got := clockTime.In(loc).Format("15:04:05 -0700 MST"); got != test.want {
				t.Errorf("parseTZ(%q) zone = %s, want %s", test.tz, got, test.want)
			}
		})
	}
}

func TestRunDateOption(t *testing.T) {
	tests := []testCase{
		{
			name: "the example of the specification",
			args: []string{"-d", "2020-07-21 14:19:13 +0530"},
			want: "Tue Jul 21 08:49:13 UTC 2020\n",
		},
		{
			name: "the format operand applies to the result",
			args: []string{"-d", "2020-07-21 14:19:13 +0530", "+%Y-%m-%d %H:%M:%S"},
			want: "2020-07-21 08:49:13\n",
		},
		{
			name: "nanoseconds with %s and %N",
			args: []string{"-d", "2020-07-21 14:19:13.489392193 +0530", "+%s.%N"},
			want: "1595321353.489392193\n",
		},
		{
			name: "a relative item",
			args: []string{"-d", "yesterday"},
			want: "Mon Jun 25 00:00:00 UTC 1990\n",
		},
		{
			name: "a time since the epoch",
			args: []string{"-d", "@646394290"},
			want: "Tue Jun 26 09:58:10 UTC 1990\n",
		},
		{
			name: "the --date long option",
			args: []string{"--date=yesterday"},
			want: "Mon Jun 25 00:00:00 UTC 1990\n",
		},
		{
			name: "the --date long option with the next argument",
			args: []string{"--date", "yesterday"},
			want: "Mon Jun 25 00:00:00 UTC 1990\n",
		},
		{
			name: "only the last -d counts",
			args: []string{"-d", "2020-01-01", "-d", "2021-01-01", "+%Y"},
			want: "2021\n",
		},
		{
			name: "the string without a zone is parsed in the TZ zone",
			env:  map[string]string{"TZ": "EST5"},
			args: []string{"-d", "2020-01-01 12:00"},
			want: "Wed Jan  1 12:00:00 EST 2020\n",
		},
		{
			name: "the zone of the string is honored, the output is in the TZ zone",
			env:  map[string]string{"TZ": "EST5"},
			args: []string{"-d", "2020-01-01 12:00 +0100", "+%Y-%m-%d %H:%M %Z"},
			want: "2020-01-01 06:00 EST\n",
		},
		{
			name: "-u displays the -d string in UTC",
			env:  map[string]string{"TZ": "EST5"},
			args: []string{"-u", "-d", "2020-01-01 12:00 +0100", "+%Y-%m-%d %H:%M %Z"},
			want: "2020-01-01 11:00 UTC\n",
		},
		{
			name:     "an invalid date string",
			args:     []string{"-d", "bogus"},
			wantErr:  "date: invalid date 'bogus'\n",
			wantExit: 1,
		},
		{
			name: "an empty date string is the beginning of today",
			args: []string{"-d", ""},
			want: "Tue Jun 26 00:00:00 UTC 1990\n",
		},
		{
			name:     "the -d option without an argument",
			args:     []string{"-d"},
			wantErr:  "date: option requires an argument -- 'd'\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a settime operand with -d",
			args:     []string{"-d", "2020-01-01", "07101230"},
			wantErr:  "date: the argument '07101230' lacks a leading '+'; when using an option to specify date(s), any non-option argument must be a format string beginning with '+'\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "-d with -r",
			args:     []string{"-d", "2020-01-01", "-r", "ref.txt"},
			fsys:     fsx.MemoryFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			wantErr:  "date: the options to specify dates for printing are mutually exclusive\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "-d with -f",
			args:     []string{"-d", "2020-01-01", "-f", "dates.txt"},
			fsys:     fsx.MemoryFS{"dates.txt": {Data: []byte("2020-01-01\n")}},
			wantErr:  "date: the options to specify dates for printing are mutually exclusive\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "-r with -f",
			args:     []string{"-r", "ref.txt", "-f", "dates.txt"},
			fsys:     fsx.MemoryFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}, "dates.txt": {Data: []byte("2020-01-01\n")}},
			wantErr:  "date: the options to specify dates for printing are mutually exclusive\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFileOption(t *testing.T) {
	tests := []testCase{
		{
			name: "one date per line",
			args: []string{"-f", "dates.txt"},
			fsys: fsx.MemoryFS{"dates.txt": {Data: []byte("2020-01-01\n2021-02-03\n")}},
			want: "Wed Jan  1 00:00:00 UTC 2020\nWed Feb  3 00:00:00 UTC 2021\n",
		},
		{
			name: "the format operand applies to every line",
			args: []string{"-f", "dates.txt", "+%Y-%m-%d"},
			fsys: fsx.MemoryFS{"dates.txt": {Data: []byte("2020-01-01\n2021-02-03\n")}},
			want: "2020-01-01\n2021-02-03\n",
		},
		{
			name: "the --file long option",
			args: []string{"--file=dates.txt"},
			fsys: fsx.MemoryFS{"dates.txt": {Data: []byte("2020-01-01\n")}},
			want: "Wed Jan  1 00:00:00 UTC 2020\n",
		},
		{
			name: "a relative line",
			args: []string{"-f", "dates.txt"},
			fsys: fsx.MemoryFS{"dates.txt": {Data: []byte("yesterday\n")}},
			want: "Mon Jun 25 00:00:00 UTC 1990\n",
		},
		{
			name: "the last line without a newline",
			args: []string{"-f", "dates.txt"},
			fsys: fsx.MemoryFS{"dates.txt": {Data: []byte("2020-01-01")}},
			want: "Wed Jan  1 00:00:00 UTC 2020\n",
		},
		{
			name:  "the lines of the standard input with -",
			args:  []string{"-f", "-"},
			stdin: strings.NewReader("2020-01-01\n2021-02-03\n"),
			want:  "Wed Jan  1 00:00:00 UTC 2020\nWed Feb  3 00:00:00 UTC 2021\n",
		},
		{
			name:     "an invalid line, after the valid ones were printed",
			args:     []string{"-f", "dates.txt"},
			fsys:     fsx.MemoryFS{"dates.txt": {Data: []byte("2020-01-01\nbogus\n2020-02-02\n")}},
			want:     "Wed Jan  1 00:00:00 UTC 2020\nSun Feb  2 00:00:00 UTC 2020\n",
			wantErr:  "date: invalid date 'bogus'\n",
			wantExit: 1,
		},
		{
			name: "an empty line is the beginning of today",
			args: []string{"-f", "dates.txt"},
			fsys: fsx.MemoryFS{"dates.txt": {Data: []byte("2020-01-01\n\n")}},
			want: "Wed Jan  1 00:00:00 UTC 2020\nTue Jun 26 00:00:00 UTC 1990\n",
		},
		{
			name:     "a file which does not exist",
			args:     []string{"-f", "missing.txt"},
			fsys:     fsx.MemoryFS{},
			wantErr:  "date: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "no file system",
			args:     []string{"-f", "dates.txt"},
			wantErr:  "date: dates.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "the -f option without an argument",
			args:     []string{"-f"},
			wantErr:  "date: option requires an argument -- 'f'\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunISO8601(t *testing.T) {
	tests := []testCase{
		{
			name: "the date only by default",
			args: []string{"-I"},
			want: "1990-06-26\n",
		},
		{
			name: "the auto timespec",
			args: []string{"-Iauto"},
			want: "1990-06-26\n",
		},
		{
			name: "the date timespec",
			args: []string{"-Idate"},
			want: "1990-06-26\n",
		},
		{
			name: "the hours timespec",
			args: []string{"-Ihours"},
			want: "1990-06-26T09+00:00\n",
		},
		{
			name: "the minutes timespec",
			args: []string{"-Iminutes"},
			want: "1990-06-26T09:58+00:00\n",
		},
		{
			name: "the seconds timespec",
			args: []string{"-Iseconds"},
			want: "1990-06-26T09:58:10+00:00\n",
		},
		{
			name: "the ns timespec",
			args: []string{"-Ins"},
			want: "1990-06-26T09:58:10,000000000+00:00\n",
		},
		{
			name: "the --iso-8601 long option",
			args: []string{"--iso-8601"},
			want: "1990-06-26\n",
		},
		{
			name: "the --iso-8601 long option with an attached argument",
			args: []string{"--iso-8601=ns"},
			want: "1990-06-26T09:58:10,000000000+00:00\n",
		},
		{
			name: "the zone of TZ",
			env:  map[string]string{"TZ": "EST5"},
			args: []string{"-Iseconds"},
			want: "1990-06-26T04:58:10-05:00\n",
		},
		{
			name: "with the -d option",
			args: []string{"-Iseconds", "-d", "2020-07-21T14:19:13+0530"},
			want: "2020-07-21T08:49:13+00:00\n",
		},
		{
			name: "nanoseconds with the -d option",
			args: []string{"-Ins", "-d", "2020-07-21 14:19:13.489392193 +0530"},
			want: "2020-07-21T08:49:13,489392193+00:00\n",
		},
		{
			name:     "with a format operand",
			args:     []string{"-I", "+%Y"},
			wantErr:  "date: multiple output formats specified\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid timespec",
			args:     []string{"-Ibogus"},
			wantErr:  "date: invalid argument 'bogus' for '--iso-8601'\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRfcEmail(t *testing.T) {
	tests := []testCase{
		{
			name: "the -R option",
			args: []string{"-R"},
			want: "Tue, 26 Jun 1990 09:58:10 +0000\n",
		},
		{
			name: "the --rfc-email long option",
			args: []string{"--rfc-email"},
			want: "Tue, 26 Jun 1990 09:58:10 +0000\n",
		},
		{
			name: "the --rfc-2822 alias",
			args: []string{"--rfc-2822"},
			want: "Tue, 26 Jun 1990 09:58:10 +0000\n",
		},
		{
			name: "the --rfc-822 alias",
			args: []string{"--rfc-822"},
			want: "Tue, 26 Jun 1990 09:58:10 +0000\n",
		},
		{
			name: "the zone of TZ",
			env:  map[string]string{"TZ": "EST5"},
			args: []string{"-R"},
			want: "Tue, 26 Jun 1990 04:58:10 -0500\n",
		},
		{
			name: "with the -d option",
			args: []string{"-R", "-d", "2020-07-21T14:19:13+0530"},
			want: "Tue, 21 Jul 2020 08:49:13 +0000\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRfc3339(t *testing.T) {
	tests := []testCase{
		{
			name: "the date timespec",
			args: []string{"--rfc-3339=date"},
			want: "1990-06-26\n",
		},
		{
			name: "the seconds timespec",
			args: []string{"--rfc-3339=seconds"},
			want: "1990-06-26 09:58:10+00:00\n",
		},
		{
			name: "the ns timespec",
			args: []string{"--rfc-3339=ns"},
			want: "1990-06-26 09:58:10.000000000+00:00\n",
		},
		{
			name: "the argument in the next argument",
			args: []string{"--rfc-3339", "seconds"},
			want: "1990-06-26 09:58:10+00:00\n",
		},
		{
			name: "the zone of TZ",
			env:  map[string]string{"TZ": "EST5"},
			args: []string{"--rfc-3339=seconds"},
			want: "1990-06-26 04:58:10-05:00\n",
		},
		{
			name: "with the -d option",
			args: []string{"--rfc-3339=ns", "-d", "2020-07-21 14:19:13.489392193 +0530"},
			want: "2020-07-21 08:49:13.489392193+00:00\n",
		},
		{
			name:     "an invalid timespec",
			args:     []string{"--rfc-3339=bogus"},
			wantErr:  "date: invalid argument 'bogus' for '--rfc-3339'\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "without an argument",
			args:     []string{"--rfc-3339"},
			wantErr:  "date: option '--rfc-3339' requires an argument\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "with a format operand",
			args:     []string{"--rfc-3339=date", "+%Y"},
			wantErr:  "date: multiple output formats specified\nTry 'date --help' for more information.\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "date", usage)
}
