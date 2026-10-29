package date

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// clockTime is the time the tests read as the current one, so their output
// doesn't depend on when they run. It is the time of an example of the
// specification.
var clockTime = time.Date(1990, 6, 26, 9, 58, 10, 0, time.UTC)

// refTime is the modification time of the reference files of the tests.
var refTime = time.Date(2020, 3, 1, 12, 30, 0, 0, time.UTC)

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type testCase struct {
	name     string
	fsys     fsx.MemFS
	env      map[string]string
	args     []string
	want     string
	wantErr  string
	wantExit int
}

// environ converts a map to a list of KEY=VALUE strings.
func environ(env map[string]string) []string {
	entries := make([]string, 0, len(env))
	for name, value := range env {
		entries = append(entries, name+"="+value)
	}
	return entries
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
		Clock:   func() time.Time { return clockTime },
		Environ: func() []string { return environ(env) },
	}
	if test.fsys != nil {
		rt.FS = test.fsys
	}
	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.want {
		t.Errorf("Run() stdout = %q, want %q", got, test.want)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
}

func exitCode(t *testing.T, err error) int {
	t.Helper()

	if err == nil {
		return 0
	}
	var exitErr *coreutils.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
	}
	return exitErr.Code
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
			fsys: fsx.MemFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "202003011230.00\n",
		},
		{
			name: "the modification time of the -r file",
			args: []string{"-r", "ref.txt"},
			fsys: fsx.MemFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "Sun Mar  1 12:30:00 UTC 2020\n",
		},
		{
			name: "the -r option with its argument glued",
			args: []string{"-rref.txt"},
			fsys: fsx.MemFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "Sun Mar  1 12:30:00 UTC 2020\n",
		},
		{
			name: "the --reference long option",
			args: []string{"--reference", "ref.txt"},
			fsys: fsx.MemFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "Sun Mar  1 12:30:00 UTC 2020\n",
		},
		{
			name: "the --reference long option with an attached argument",
			args: []string{"--reference=ref.txt"},
			fsys: fsx.MemFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "Sun Mar  1 12:30:00 UTC 2020\n",
		},
		{
			name: "the -r file in a subdirectory",
			args: []string{"-r", "sub/ref.txt"},
			fsys: fsx.MemFS{"sub/ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "Sun Mar  1 12:30:00 UTC 2020\n",
		},
		{
			name: "the -r file with a format operand",
			args: []string{"-r", "ref.txt", "+%Y"},
			fsys: fsx.MemFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "2020\n",
		},
		{
			name: "the -r file with -u",
			args: []string{"-ur", "ref.txt"},
			fsys: fsx.MemFS{"ref.txt": {Data: []byte("x"), ModTime: refTime}},
			want: "Sun Mar  1 12:30:00 UTC 2020\n",
		},
		{
			name:     "the -r option with an empty argument",
			args:     []string{"-r", ""},
			fsys:     fsx.MemFS{},
			wantErr:  "date: failed to get timestamp of '': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "the --reference option with an empty attached argument",
			args:     []string{"--reference="},
			fsys:     fsx.MemFS{},
			wantErr:  "date: failed to get timestamp of '': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a -r file which does not exist",
			args:     []string{"-r", "missing.txt"},
			fsys:     fsx.MemFS{},
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
			wantErr:  "date: option requires an argument -- 'r'\n",
			wantExit: 1,
		},
		{
			name:     "an invalid option",
			args:     []string{"-x"},
			wantErr:  "date: invalid option -- 'x'\n",
			wantExit: 1,
		},
		{
			name:     "an unrecognized long option",
			args:     []string{"--bogus"},
			wantErr:  "date: unrecognized option '--bogus'\n",
			wantExit: 1,
		},
		{
			name:     "an extra operand",
			args:     []string{"+%Y", "extra"},
			wantErr:  "date: extra operand 'extra'\n",
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
			Stdout: errorWriter{err: errors.New("boom")},
			Stderr: new(bytes.Buffer),
			Clock:  func() time.Time { return clockTime },
		}
		err := New(rt).Run(t.Context(), "+%Y")
		if code := exitCode(t, err); code != 1 {
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
