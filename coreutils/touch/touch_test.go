package touch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// timePair holds the expected access and modification times of a file. A zero
// time means the test only checks that it's around the time of the run.
type timePair struct {
	atime time.Time
	mtime time.Time
}

// both returns a time pair where the access and modification times are the
// same.
func both(t time.Time) timePair {
	return timePair{atime: t, mtime: t}
}

type testCase struct {
	name     string
	files    fsx.MemFS
	tz       string
	args     []string
	want     map[string]timePair
	absent   []string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: stderr,
		FS:     test.files,
		Environ: func() []string {
			if test.tz == "" {
				return nil
			}
			return []string{"TZ=" + test.tz}
		},
	}

	before := time.Now()
	err := New(rt).Run(t.Context(), test.args...)
	after := time.Now()

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}

	for name, want := range test.want {
		fi, err := fs.Stat(test.files, name)
		if err != nil {
			t.Errorf("Stat(%q) error = %v", name, err)
			continue
		}
		got, _ := fsx.Atime(fi)
		assertTime(t, name, "atime", got, want.atime, before, after)
		assertTime(t, name, "mtime", fi.ModTime(), want.mtime, before, after)
	}
	for _, name := range test.absent {
		if _, err := fs.Stat(test.files, name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat(%q) error = %v, want fs.ErrNotExist", name, err)
		}
	}
}

// assertTime checks a time that is either exact, when want is not zero, or
// within the time the run took, when want is zero.
func assertTime(t *testing.T, name, label string, got, want, before, after time.Time) {
	t.Helper()

	if want.IsZero() {
		if got.Before(before) || got.After(after) {
			t.Errorf("%s %s = %v, want between %v and %v", name, label, got, before, after)
		}
		return
	}
	if !got.Equal(want) {
		t.Errorf("%s %s = %v, want %v", name, label, got, want)
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

var (
	refAtime = time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	refMtime = time.Date(2021, 6, 7, 8, 9, 10, 0, time.UTC)
)

// testFS returns a file system holding a reference file with distinct access
// and modification times.
func testFS() fsx.MemFS {
	fsys := fsx.MemFS{
		"ref.txt":      {Data: []byte("ref")},
		"sub/keep.txt": {Data: []byte("keep")},
	}
	if err := fsys.Chtimes("ref.txt", refAtime, refMtime); err != nil {
		panic(err)
	}
	return fsys
}

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			name:  "creates a missing file",
			files: fsx.MemFS{},
			args:  []string{"file.txt"},
			want:  map[string]timePair{"file.txt": {}},
		},
		{
			name:  "creates every missing file",
			files: fsx.MemFS{},
			args:  []string{"one.txt", "two.txt"},
			want:  map[string]timePair{"one.txt": {}, "two.txt": {}},
		},
		{
			name:  "creates a file inside an existing directory",
			files: fsx.MemFS{"sub/keep.txt": {Data: []byte("keep")}},
			args:  []string{"sub/new.txt"},
			want:  map[string]timePair{"sub/new.txt": {}},
		},
		{
			name:  "updates the times of an existing file",
			files: fsx.MemFS{"file.txt": {Data: []byte("hello")}},
			args:  []string{"file.txt"},
			want:  map[string]timePair{"file.txt": {}},
		},
		{
			name:  "a file named like an option after a double dash",
			files: fsx.MemFS{},
			args:  []string{"--", "-t"},
			want:  map[string]timePair{"-t": {}},
		},
		{
			name:     "a missing parent directory is an error",
			files:    fsx.MemFS{},
			args:     []string{"missing/file.txt"},
			wantErr:  "touch: missing/file.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "keeps touching the other files after an error",
			files:    fsx.MemFS{},
			args:     []string{"missing/file.txt", "ok.txt"},
			want:     map[string]timePair{"ok.txt": {}},
			wantErr:  "touch: missing/file.txt: file does not exist\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunAccessTime(t *testing.T) {
	tests := []testCase{
		{
			name:  "a sets the access time only",
			files: testFS(),
			tz:    "UTC",
			args:  []string{"-a", "-t", "202201020304.05", "ref.txt"},
			want:  map[string]timePair{"ref.txt": {atime: time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC), mtime: refMtime}},
		},
		{
			name:  "a on a new file keeps the creation modification time",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-a", "-t", "202201020304.05", "new.txt"},
			want:  map[string]timePair{"new.txt": {atime: time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC)}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunModificationTime(t *testing.T) {
	tests := []testCase{
		{
			name:  "m sets the modification time only",
			files: testFS(),
			tz:    "UTC",
			args:  []string{"-m", "-t", "202201020304.05", "ref.txt"},
			want:  map[string]timePair{"ref.txt": {atime: refAtime, mtime: time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC)}},
		},
		{
			name:  "a and m are the default",
			files: testFS(),
			tz:    "UTC",
			args:  []string{"-am", "-t", "202201020304.05", "ref.txt"},
			want:  map[string]timePair{"ref.txt": both(time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC))},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunNoCreate(t *testing.T) {
	tests := []testCase{
		{
			name:   "c does not create the file",
			files:  fsx.MemFS{},
			args:   []string{"-c", "missing.txt"},
			absent: []string{"missing.txt"},
		},
		{
			name:   "c does not create the file with a time",
			files:  fsx.MemFS{},
			tz:     "UTC",
			args:   []string{"-c", "-t", "202201020304.05", "missing.txt"},
			absent: []string{"missing.txt"},
		},
		{
			name:  "c still updates existing files",
			files: testFS(),
			tz:    "UTC",
			args:  []string{"-c", "-t", "202201020304.05", "ref.txt"},
			want:  map[string]timePair{"ref.txt": both(time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC))},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunReferenceFile(t *testing.T) {
	tests := []testCase{
		{
			name:  "r uses both times of the reference file",
			files: testFS(),
			args:  []string{"-r", "ref.txt", "file.txt"},
			want:  map[string]timePair{"file.txt": {atime: refAtime, mtime: refMtime}},
		},
		{
			name:  "r with a creates the file with the reference access time",
			files: testFS(),
			args:  []string{"-a", "-r", "ref.txt", "file.txt"},
			want:  map[string]timePair{"file.txt": {atime: refAtime}},
		},
		{
			name:  "r with m updates the modification time of the reference file",
			files: testFS(),
			args:  []string{"-m", "-r", "ref.txt", "ref.txt"},
			want:  map[string]timePair{"ref.txt": {atime: refAtime, mtime: refMtime}},
		},
		{
			name:     "a missing reference file is an error",
			files:    fsx.MemFS{},
			args:     []string{"-r", "missing.txt", "file.txt"},
			absent:   []string{"file.txt"},
			wantErr:  "touch: missing.txt: file does not exist\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunTimeOption(t *testing.T) {
	tests := []testCase{
		{
			name:  "CCYYMMDDhhmm",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-t", "202201020304", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2022, 1, 2, 3, 4, 0, 0, time.UTC))},
		},
		{
			name:  "YYMMDDhhmm in the 20th century",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-t", "7001020304", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(1970, 1, 2, 3, 4, 0, 0, time.UTC))},
		},
		{
			name:  "YYMMDDhhmm in the 21st century",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-t", "0101020304", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2001, 1, 2, 3, 4, 0, 0, time.UTC))},
		},
		{
			name:  "MMDDhhmm uses the current year",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-t", "01020304", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(time.Now().Year(), 1, 2, 3, 4, 0, 0, time.UTC))},
		},
		{
			name:  "seconds",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-t", "202201020304.05", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC))},
		},
		{
			name:  "a leap second is one second later",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-t", "202201020304.60", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2022, 1, 2, 3, 5, 0, 0, time.UTC))},
		},
		{
			name:  "a glued option-argument",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-t202201020304.05", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC))},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDateTimeOption(t *testing.T) {
	tests := []testCase{
		{
			name:  "date and time",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-d", "2007-11-12T10:15:30", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2007, 11, 12, 10, 15, 30, 0, time.UTC))},
		},
		{
			name:  "a space instead of the T designator",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-d", "2007-11-12 10:15:30", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2007, 11, 12, 10, 15, 30, 0, time.UTC))},
		},
		{
			name:  "a UTC time",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-d", "2007-11-12T10:15:30Z", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2007, 11, 12, 10, 15, 30, 0, time.UTC))},
		},
		{
			name:  "a fractional second with a period",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-d", "2007-11-12T10:15:30.002", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2007, 11, 12, 10, 15, 30, 2000000, time.UTC))},
		},
		{
			name:  "a fractional second with a comma",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"-d", "2007-11-12T10:15:30,002", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2007, 11, 12, 10, 15, 30, 2000000, time.UTC))},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLastTimeSourceWins(t *testing.T) {
	tests := []testCase{
		{
			name:  "a time after a reference file",
			files: testFS(),
			tz:    "UTC",
			args:  []string{"-r", "ref.txt", "-t", "202201020304.05", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC))},
		},
		{
			name:  "a reference file after a time",
			files: testFS(),
			tz:    "UTC",
			args:  []string{"-t", "202201020304.05", "-r", "ref.txt", "file.txt"},
			want:  map[string]timePair{"file.txt": {atime: refAtime, mtime: refMtime}},
		},
		{
			name:  "a date after a reference file",
			files: testFS(),
			tz:    "UTC",
			args:  []string{"-r", "ref.txt", "-d", "2007-11-12T10:15:30", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2007, 11, 12, 10, 15, 30, 0, time.UTC))},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLongOptions(t *testing.T) {
	tests := []testCase{
		{
			name:  "time atime sets the access time only",
			files: testFS(),
			tz:    "UTC",
			args:  []string{"--time=atime", "-t", "202201020304.05", "ref.txt"},
			want:  map[string]timePair{"ref.txt": {atime: time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC), mtime: refMtime}},
		},
		{
			name:  "time mtime sets the modification time only",
			files: testFS(),
			tz:    "UTC",
			args:  []string{"--time=mtime", "-t", "202201020304.05", "ref.txt"},
			want:  map[string]timePair{"ref.txt": {atime: refAtime, mtime: time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC)}},
		},
		{
			name:   "no-create does not create the file",
			files:  fsx.MemFS{},
			args:   []string{"--no-create", "missing.txt"},
			absent: []string{"missing.txt"},
		},
		{
			name:  "date sets both times",
			files: fsx.MemFS{},
			tz:    "UTC",
			args:  []string{"--date", "2007-11-12T10:15:30", "file.txt"},
			want:  map[string]timePair{"file.txt": both(time.Date(2007, 11, 12, 10, 15, 30, 0, time.UTC))},
		},
		{
			name:  "reference uses both times of the reference file",
			files: testFS(),
			args:  []string{"--reference=ref.txt", "file.txt"},
			want:  map[string]timePair{"file.txt": {atime: refAtime, mtime: refMtime}},
		},
		{
			name:     "an unknown time word is an error",
			files:    fsx.MemFS{},
			args:     []string{"--time=bogus", "file.txt"},
			wantErr:  "touch: invalid argument 'bogus' for '--time'\n",
			wantExit: 2,
		},
		{
			name:     "time requires an argument",
			files:    fsx.MemFS{},
			args:     []string{"--time"},
			wantErr:  "touch: option '--time' requires an argument\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "no operands",
			wantErr: "touch: missing file operand\n",
		},
		{
			name:    "only options",
			args:    []string{"-a"},
			wantErr: "touch: missing file operand\n",
		},
		{
			name:    "invalid option",
			args:    []string{"-x", "file.txt"},
			wantErr: "touch: invalid option -- 'x'\n",
		},
		{
			name:    "invalid grouped option",
			args:    []string{"-ax", "file.txt"},
			wantErr: "touch: invalid option -- 'x'\n",
		},
		{
			name:    "unrecognized long option",
			args:    []string{"--all", "file.txt"},
			wantErr: "touch: unrecognized option '--all'\n",
		},
		{
			name:    "missing r argument",
			args:    []string{"-r"},
			wantErr: "touch: option requires an argument -- 'r'\n",
		},
		{
			name:    "missing t argument",
			args:    []string{"-t"},
			wantErr: "touch: option requires an argument -- 't'\n",
		},
		{
			name:    "missing d argument",
			args:    []string{"-d"},
			wantErr: "touch: option requires an argument -- 'd'\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, testCase{
				files:    fsx.MemFS{},
				args:     test.args,
				wantErr:  test.wantErr,
				wantExit: 2,
			})
		})
	}
}

func TestRunInvalidTimes(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "t with a bad length",
			args:    []string{"-t", "20220102030"},
			wantErr: "touch: invalid date format \"20220102030\"\n",
		},
		{
			name:    "t with non-digits",
			args:    []string{"-t", "2022-10-20"},
			wantErr: "touch: invalid date format \"2022-10-20\"\n",
		},
		{
			name:    "t with a bad month",
			args:    []string{"-t", "202213020304"},
			wantErr: "touch: invalid date format \"202213020304\"\n",
		},
		{
			name:    "t with a bad day",
			args:    []string{"-t", "202201000304"},
			wantErr: "touch: invalid date format \"202201000304\"\n",
		},
		{
			name:    "t with a bad hour",
			args:    []string{"-t", "202201022404"},
			wantErr: "touch: invalid date format \"202201022404\"\n",
		},
		{
			name:    "t with a bad minute",
			args:    []string{"-t", "202201020360"},
			wantErr: "touch: invalid date format \"202201020360\"\n",
		},
		{
			name:    "t with a bad second",
			args:    []string{"-t", "202201020304.61"},
			wantErr: "touch: invalid date format \"202201020304.61\"\n",
		},
		{
			name:    "t with a one digit second",
			args:    []string{"-t", "202201020304.5"},
			wantErr: "touch: invalid date format \"202201020304.5\"\n",
		},
		{
			name:    "d with a bad date",
			args:    []string{"-d", "not-a-date"},
			wantErr: "touch: invalid date format \"not-a-date\"\n",
		},
		{
			name:    "d with a missing seconds field",
			args:    []string{"-d", "2007-11-12T10:15"},
			wantErr: "touch: invalid date format \"2007-11-12T10:15\"\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, testCase{
				files:    fsx.MemFS{},
				tz:       "UTC",
				args:     append(test.args, "file.txt"),
				absent:   []string{"file.txt"},
				wantErr:  test.wantErr,
				wantExit: 1,
			})
		})
	}
}

func TestRunEmptyOperand(t *testing.T) {
	runCase(t, testCase{
		files:    fsx.MemFS{},
		args:     []string{""},
		wantErr:  "touch: : invalid argument\n",
		wantExit: 1,
	})
}

func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rt := &coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: new(bytes.Buffer),
		FS:     fsx.MemFS{},
	}
	err := New(rt).Run(ctx, "file.txt")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRunNoFilesystem(t *testing.T) {
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context(), "file.txt")

	if code := exitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1", code)
	}
	if got, want := stderr.String(), "touch: no file system available\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}

func TestParseTime(t *testing.T) {
	now := time.Date(2022, 6, 7, 8, 9, 10, 0, time.UTC)

	tests := []struct {
		name    string
		s       string
		want    time.Time
		wantErr bool
	}{
		{
			name: "MMDDhhmm",
			s:    "01020304",
			want: time.Date(2022, 1, 2, 3, 4, 0, 0, time.UTC),
		},
		{
			name: "YYMMDDhhmm in the 20th century",
			s:    "9901020304",
			want: time.Date(1999, 1, 2, 3, 4, 0, 0, time.UTC),
		},
		{
			name: "YYMMDDhhmm in the 21st century",
			s:    "0001020304",
			want: time.Date(2000, 1, 2, 3, 4, 0, 0, time.UTC),
		},
		{
			name: "CCYYMMDDhhmm",
			s:    "202201020304",
			want: time.Date(2022, 1, 2, 3, 4, 0, 0, time.UTC),
		},
		{
			name: "seconds",
			s:    "202201020304.59",
			want: time.Date(2022, 1, 2, 3, 4, 59, 0, time.UTC),
		},
		{
			name: "sixty seconds",
			s:    "202201020304.60",
			want: time.Date(2022, 1, 2, 3, 5, 0, 0, time.UTC),
		},
		{name: "empty", s: "", wantErr: true},
		{name: "too short", s: "0102", wantErr: true},
		{name: "too long", s: "2022010203045", wantErr: true},
		{name: "month 0", s: "202200020304", wantErr: true},
		{name: "month 13", s: "202213020304", wantErr: true},
		{name: "day 0", s: "202201000304", wantErr: true},
		{name: "day 32", s: "202201320304", wantErr: true},
		{name: "hour 24", s: "202201022404", wantErr: true},
		{name: "minute 60", s: "202201020360", wantErr: true},
		{name: "second 61", s: "202201020304.61", wantErr: true},
		{name: "one digit second", s: "202201020304.5", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseTime(test.s, time.UTC, now)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseTime(%q) error = nil, want an error", test.s)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTime(%q) error = %v", test.s, err)
			}
			if !got.Equal(test.want) {
				t.Errorf("parseTime(%q) = %v, want %v", test.s, got, test.want)
			}
		})
	}
}

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantOpts  options
		wantFiles []string
	}{
		{
			name:      "flags for every letter",
			args:      []string{"-acm", "file.txt"},
			wantOpts:  options{access: true, modify: true, noCreate: true},
			wantFiles: []string{"file.txt"},
		},
		{
			name:      "individual flags",
			args:      []string{"-a", "-m", "file.txt"},
			wantOpts:  options{access: true, modify: true},
			wantFiles: []string{"file.txt"},
		},
		{
			name:      "access and modification are the default",
			args:      []string{"file.txt"},
			wantOpts:  options{access: true, modify: true},
			wantFiles: []string{"file.txt"},
		},
		{
			name:      "a lone dash is an operand",
			args:      []string{"-"},
			wantOpts:  options{access: true, modify: true},
			wantFiles: []string{"-"},
		},
		{
			name:      "options stop at the first operand",
			args:      []string{"file.txt", "-a"},
			wantOpts:  options{access: true, modify: true},
			wantFiles: []string{"file.txt", "-a"},
		},
		{
			name:      "a double dash ends the options",
			args:      []string{"--", "-a"},
			wantOpts:  options{access: true, modify: true},
			wantFiles: []string{"-a"},
		},
		{
			name:      "a glued option-argument",
			args:      []string{"-t202201020304"},
			wantOpts:  options{access: true, modify: true, source: 't', value: "202201020304"},
			wantFiles: nil,
		},
		{
			name:      "a separate option-argument",
			args:      []string{"-r", "ref.txt"},
			wantOpts:  options{access: true, modify: true, source: 'r', value: "ref.txt"},
			wantFiles: nil,
		},
		{
			name:      "an option-argument glued to a group",
			args:      []string{"-cd", "2007-11-12T10:15:30"},
			wantOpts:  options{access: true, modify: true, noCreate: true, source: 'd', value: "2007-11-12T10:15:30"},
			wantFiles: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotOpts, gotFiles, err := parseFlags(test.args)
			if err != nil {
				t.Fatalf("parseFlags(%q) error = %v", test.args, err)
			}
			if gotOpts != test.wantOpts {
				t.Errorf("parseFlags(%q) options = %+v, want %+v", test.args, gotOpts, test.wantOpts)
			}
			if len(gotFiles) != len(test.wantFiles) {
				t.Fatalf("parseFlags(%q) files = %q, want %q", test.args, gotFiles, test.wantFiles)
			}
			for i, got := range gotFiles {
				if got != test.wantFiles[i] {
					t.Errorf("parseFlags(%q) files = %q, want %q", test.args, gotFiles, test.wantFiles)
				}
			}
		})
	}
}
