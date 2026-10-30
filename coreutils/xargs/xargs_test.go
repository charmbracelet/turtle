package xargs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// execRecorder records the commands run by Run, replying with the given
// statuses, or with an error from the given index on.
type execRecorder struct {
	commands    []string
	statuses    []int
	err         error
	errorStatus int
}

func (e *execRecorder) exec(_ context.Context, name string, args, env []string) (int, error) {
	e.commands = append(e.commands, strings.Join(append([]string{name}, args...), " "))
	if e.err != nil && len(e.commands) > len(e.statuses) {
		status := e.errorStatus
		if status == 0 {
			status = 126
		}
		return status, e.err
	}
	if len(e.statuses) < len(e.commands) {
		return 0, nil
	}
	return e.statuses[len(e.commands)-1], nil
}

// errorReader fails reads with the given error.
type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
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

type testCase struct {
	name     string
	stdin    io.Reader
	tty      io.Reader
	fsys     fsx.FS
	args     []string
	exec     *execRecorder
	wantCmds []string
	wantErr  string
	wantExit int
}

func (tc testCase) run(t *testing.T) {
	t.Helper()

	t.Parallel()
	var stderr strings.Builder
	p := New(&coreutils.Runtime{
		Stdin:  tc.stdin,
		Stdout: io.Discard,
		Stderr: &stderr,
		FS:     tc.fsys,
		Exec:   tc.exec.exec,
	}).(*Prog)
	p.tty = tc.tty
	err := p.Run(t.Context(), tc.args...)

	var gotCmds []string
	if tc.exec != nil {
		gotCmds = tc.exec.commands
	}
	if strings.Join(gotCmds, "\n") != strings.Join(tc.wantCmds, "\n") {
		t.Errorf("commands = %q, want %q", gotCmds, tc.wantCmds)
	}
	if got := stderr.String(); got != tc.wantErr {
		t.Errorf("stderr = %q, want %q", got, tc.wantErr)
	}
	if code := exitCode(t, err); code != tc.wantExit {
		t.Errorf("exit code = %d, want %d", code, tc.wantExit)
	}
}

func newExec(statuses ...int) *execRecorder {
	return &execRecorder{statuses: statuses}
}

func TestRun(t *testing.T) {
	tests := []testCase{
		{
			name:     "default utility is echo",
			stdin:    strings.NewReader("a b\n"),
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
		{
			name:     "utility with initial arguments",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"--", "printf", "[%s]"},
			exec:     newExec(),
			wantCmds: []string{"printf [%s] a b"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestTokenize(t *testing.T) {
	tests := []testCase{
		{
			name:     "blanks and newlines delimit",
			stdin:    strings.NewReader("a b\nc\t d  \n\n"),
			exec:     newExec(),
			wantCmds: []string{"echo a b c d"},
		},
		{
			name:     "adjacent delimiters are a single one",
			stdin:    strings.NewReader("  a \t\n\n b \n"),
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
		{
			name:     "double quotes",
			stdin:    strings.NewReader("a \"b c\" \"d\"e\n"),
			exec:     newExec(),
			wantCmds: []string{"echo a b c de"},
		},
		{
			name:     "apostrophes",
			stdin:    strings.NewReader("a 'b c' 'd'e\n"),
			exec:     newExec(),
			wantCmds: []string{"echo a b c de"},
		},
		{
			name:     "quotes can't quote a newline",
			stdin:    strings.NewReader("\"a\nb\" c\n"),
			exec:     newExec(),
			wantCmds: []string{"echo a b c"},
		},
		{
			name:     "backslash escapes any character",
			stdin:    strings.NewReader(`a\ b \"c\" \'d\' \\ e` + "\n"),
			exec:     newExec(),
			wantCmds: []string{`echo a b "c" 'd' \ e`},
		},
		{
			name:     "empty argument from quotes",
			stdin:    strings.NewReader("a \"\" b\n"),
			exec:     newExec(),
			wantCmds: []string{"echo a  b"},
		},
		{
			name:     "last line without newline is used",
			stdin:    strings.NewReader("a\nb"),
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestEOFString(t *testing.T) {
	tests := []testCase{
		{
			name:     "no end-of-file string by default",
			stdin:    strings.NewReader("a\n_\nb\n"),
			exec:     newExec(),
			wantCmds: []string{"echo a _ b"},
		},
		{
			name:     "checked after quote processing",
			stdin:    strings.NewReader("a \"_\" b\n"),
			args:     []string{"-E", "_"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
		},
		{
			name:     "custom string",
			stdin:    strings.NewReader("a\nEND\nb\n"),
			args:     []string{"-E", "END"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
		},
		{
			name:     "empty string disables it",
			stdin:    strings.NewReader("a\n_\nb\n"),
			args:     []string{"-E", ""},
			exec:     newExec(),
			wantCmds: []string{"echo a _ b"},
		},
		{
			name:     "underscore taken literally when disabled",
			stdin:    strings.NewReader("_\n"),
			args:     []string{"-E", ""},
			exec:     newExec(),
			wantCmds: []string{"echo _"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestNullDelimiter(t *testing.T) {
	tests := []testCase{
		{
			name:     "arguments delimited by null bytes",
			stdin:    strings.NewReader("a b\x00c\x00"),
			args:     []string{"-0"},
			exec:     newExec(),
			wantCmds: []string{"echo a b c"},
		},
		{
			name:     "blanks are not delimiters",
			stdin:    strings.NewReader("a b\x00"),
			args:     []string{"-0"},
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
		{
			name:     "each null byte is a delimiter",
			stdin:    strings.NewReader("a\x00\x00b\x00"),
			args:     []string{"-0"},
			exec:     newExec(),
			wantCmds: []string{"echo a  b"},
		},
		{
			name:     "trailing non-null bytes are ignored",
			stdin:    strings.NewReader("a\x00partial"),
			args:     []string{"-0"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
		},
		{
			name:     "adjacent nulls before trailing bytes still delimit",
			stdin:    strings.NewReader("a\x00\x00partial"),
			args:     []string{"-0"},
			exec:     newExec(),
			wantCmds: []string{"echo a "},
		},
		{
			name:     "input without any null byte",
			stdin:    strings.NewReader("partial"),
			args:     []string{"-0"},
			exec:     newExec(),
			wantCmds: []string{"echo"},
		},
		{
			name:     "no end-of-file string",
			stdin:    strings.NewReader("_\x00a\x00"),
			args:     []string{"-0"},
			exec:     newExec(),
			wantCmds: []string{"echo _ a"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestInsertMode(t *testing.T) {
	tests := []testCase{
		{
			name:     "one invocation per line",
			stdin:    strings.NewReader("a\nb c\n"),
			args:     []string{"-I", "{}", "echo", "pre-{}", "post"},
			exec:     newExec(),
			wantCmds: []string{"echo pre-a post", "echo pre-b c post"},
		},
		{
			name:     "arguments appended when replacement string is absent",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-I", "{}", "echo", "pre"},
			exec:     newExec(),
			wantCmds: []string{"echo pre a"},
		},
		{
			name:     "leading blanks ignored",
			stdin:    strings.NewReader("   a\n"),
			args:     []string{"-I", "{}", "echo", "[{}]"},
			exec:     newExec(),
			wantCmds: []string{"echo [a]"},
		},
		{
			name:     "blanks are not delimiters",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"-I", "{}", "echo", "[{}]"},
			exec:     newExec(),
			wantCmds: []string{"echo [a b]"},
		},
		{
			name:     "empty lines are ignored",
			stdin:    strings.NewReader("a\n\nb\n"),
			args:     []string{"-I", "{}", "echo", "[{}]"},
			exec:     newExec(),
			wantCmds: []string{"echo [a]", "echo [b]"},
		},
		{
			name:     "whitespace-only line is an empty argument",
			stdin:    strings.NewReader("a\n \nb\n"),
			args:     []string{"-I", "{}", "echo", "[{}]"},
			exec:     newExec(),
			wantCmds: []string{"echo [a]", "echo []", "echo [b]"},
		},
		{
			name:     "no quote processing",
			stdin:    strings.NewReader("\"a b\"\n"),
			args:     []string{"-I", "{}", "echo", "[{}]"},
			exec:     newExec(),
			wantCmds: []string{`echo ["a b"]`},
		},
		{
			name:     "escaped newline is not a delimiter",
			stdin:    strings.NewReader("a\\\nb\n"),
			args:     []string{"-I", "{}", "echo", "[{}]"},
			exec:     newExec(),
			wantCmds: []string{"echo [a\\\nb]"},
		},
		{
			name:     "even backslash run doesn't escape a newline",
			stdin:    strings.NewReader("a\\\\\nb\n"),
			args:     []string{"-I", "{}", "echo", "[{}]"},
			exec:     newExec(),
			wantCmds: []string{"echo [a\\\\]", "echo [b]"},
		},
		{
			name:     "end-of-file string stops the reading",
			stdin:    strings.NewReader("a\n_\nb\n"),
			args:     []string{"-E", "_", "-I", "{}", "echo", "[{}]"},
			exec:     newExec(),
			wantCmds: []string{"echo [a]"},
		},
		{
			name:     "null delimited with -0",
			stdin:    strings.NewReader("a b\x00c\x00"),
			args:     []string{"-0", "-I", "{}", "echo", "[{}]"},
			exec:     newExec(),
			wantCmds: []string{"echo [a b]", "echo [c]"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestLinesMode(t *testing.T) {
	tests := []testCase{
		{
			name:     "one argument per line",
			stdin:    strings.NewReader("a\nb c\n"),
			args:     []string{"-L", "2", "echo", "x"},
			exec:     newExec(),
			wantCmds: []string{"echo x a b c"},
		},
		{
			name:     "last invocation with fewer arguments",
			stdin:    strings.NewReader("a\nb\nc\n"),
			args:     []string{"-L", "2", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b", "echo c"},
		},
		{
			name:     "empty lines are ignored",
			stdin:    strings.NewReader("a\n\n\nb\n"),
			args:     []string{"-L", "2", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
		{
			name:     "whitespace-only lines continue on the next non-empty line",
			stdin:    strings.NewReader(" \nb\n"),
			args:     []string{"-L", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo b"},
		},
		{
			name:     "trailing blanks continue on the next non-blank line",
			stdin:    strings.NewReader("a  \n\nb\n"),
			args:     []string{"-L", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo ab"},
		},
		{
			name:     "trailing blanks at the end of input",
			stdin:    strings.NewReader("a \n"),
			args:     []string{"-L", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
		},
		{
			name:     "null delimited with -0",
			stdin:    strings.NewReader("a b\nc\x00d e\x00"),
			args:     []string{"-0", "-L", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b\nc", "echo d e"},
		},
		{
			name:     "no quote processing",
			stdin:    strings.NewReader("\"a b\"\n"),
			args:     []string{"-L", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{`echo "a b"`},
		},
		{
			name:     "escaped trailing blank is kept",
			stdin:    strings.NewReader("a\\ \n"),
			args:     []string{"-L", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a\\ "},
		},
		{
			name:     "even backslash run doesn't escape a trailing blank",
			stdin:    strings.NewReader("a\\\\ \nb\n"),
			args:     []string{"-L", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a\\\\b"},
		},
		{
			name:     "end-of-file string stops the reading",
			stdin:    strings.NewReader("a\n_\nb\n"),
			args:     []string{"-E", "_", "-L", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestArgsPerCommand(t *testing.T) {
	tests := []testCase{
		{
			name:     "up to number arguments per invocation",
			stdin:    strings.NewReader("a b c d\n"),
			args:     []string{"-n", "2", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b", "echo c d"},
		},
		{
			name:     "last invocation with fewer arguments",
			stdin:    strings.NewReader("a b c\n"),
			args:     []string{"-n", "2", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b", "echo c"},
		},
		{
			name:     "counting includes quotes",
			stdin:    strings.NewReader("\"a b\" c d\n"),
			args:     []string{"-n", "2", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b c", "echo d"},
		},
		{
			name:     "-I wins over an earlier -n",
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"-n", "2", "-I", "{}", "echo", "[{}]"},
			exec:     newExec(),
			wantCmds: []string{"echo [a]", "echo [b]"},
		},
		{
			name:     "-n wins over an earlier -L",
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"-L", "1", "-n", "2", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
		{
			name: "-n without -s uses the LINE_MAX size",
			stdin: strings.NewReader(
				strings.Repeat("a", 2000) + " " + strings.Repeat("b", 2000) + "\n"),
			args: []string{"-n", "2", "echo"},
			exec: newExec(),
			wantCmds: []string{
				"echo " + strings.Repeat("a", 2000),
				"echo " + strings.Repeat("b", 2000),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestNoArguments(t *testing.T) {
	tests := []testCase{
		{
			name:     "empty input runs the utility once",
			stdin:    strings.NewReader(""),
			exec:     newExec(),
			wantCmds: []string{"echo"},
		},
		{
			name:     "only delimiters runs the utility once",
			stdin:    strings.NewReader(" \n\n"),
			exec:     newExec(),
			wantCmds: []string{"echo"},
		},
		{
			name:  "-r skips the invocation",
			stdin: strings.NewReader(""),
			args:  []string{"-r"},
			exec:  newExec(),
		},
		{
			name:     "-r runs the utility when there are arguments",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-r"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
		},
		{
			name:  "-r with only the end-of-file string skips the invocation",
			stdin: strings.NewReader("_\n"),
			args:  []string{"-r", "-E", "_"},
			exec:  newExec(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestSizeLimit(t *testing.T) {
	// "echo x " + 2: the base command line is 8 bytes ("echo\x00x\x00").
	tests := []testCase{
		{
			name:     "arguments fit as many as the size",
			stdin:    strings.NewReader("aa bb\n"),
			args:     []string{"-s", "13", "echo", "x"},
			exec:     newExec(),
			wantCmds: []string{"echo x aa", "echo x bb"},
		},
		{
			name:     "single argument that doesn't fit",
			stdin:    strings.NewReader("aaaa\n"),
			args:     []string{"-s", "10", "echo"},
			exec:     newExec(),
			wantErr:  "xargs: argument line too long\n",
			wantExit: 1,
		},
		{
			name:     "a later argument that doesn't fit still runs the earlier ones",
			stdin:    strings.NewReader("a\nbbbbbb\n"),
			args:     []string{"-n", "1", "-s", "10", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
			wantErr:  "xargs: argument line too long\n",
			wantExit: 1,
		},
		{
			name:     "values larger than supported are capped",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-s", "999999999"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
		},
		{
			name:     "values beyond the integer range are capped",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-s", "99999999999999999999999999999"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
		},
		{
			name:     "size too small for the command itself",
			stdin:    strings.NewReader(""),
			args:     []string{"-s", "2"},
			exec:     newExec(),
			wantErr:  "xargs: argument line too long\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestExitIfBig(t *testing.T) {
	tests := []testCase{
		{
			name:     "group smaller than -n while arguments remain",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"-n", "2", "-s", "9", "-x", "echo"},
			exec:     newExec(),
			wantErr:  "xargs: command line too long\n",
			wantExit: 1,
		},
		{
			name:     "smaller group is fine without -x",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"-n", "2", "-s", "9", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a", "echo b"},
		},
		{
			name:     "last group smaller than -n is fine",
			stdin:    strings.NewReader("a b c\n"),
			args:     []string{"-n", "2", "-s", "12", "-x", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b", "echo c"},
		},
		{
			name:     "-I forces -x on",
			stdin:    strings.NewReader("aaaa\n"),
			args:     []string{"-s", "10", "-I", "{}", "echo", "{}"},
			exec:     newExec(),
			wantErr:  "xargs: argument line too long\n",
			wantExit: 1,
		},
		{
			name:     "implied -x is cleared when -n supersedes -I",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"-I", "{}", "-n", "2", "-s", "9", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a", "echo b"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestTrace(t *testing.T) {
	tests := []testCase{
		{
			name:     "each command is written to standard error",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"-t", "-n", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a", "echo b"},
			wantErr:  "echo a\necho b\n",
		},
		{
			name:     "initial arguments are included",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-t", "printf", "%s\\n"},
			exec:     newExec(),
			wantCmds: []string{"printf %s\\n a"},
			wantErr:  "printf %s\\n a\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestPrompt(t *testing.T) {
	tests := []testCase{
		{
			name:     "affirmative response runs the command",
			stdin:    strings.NewReader("a b\n"),
			tty:      strings.NewReader("y\nn\n"),
			args:     []string{"-p", "-n", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
			wantErr:  "echo a ?...\necho b ?...\n",
		},
		{
			name:     "uppercase response runs the command",
			stdin:    strings.NewReader("a\n"),
			tty:      strings.NewReader("Y\n"),
			args:     []string{"-p", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
			wantErr:  "echo a ?...\n",
		},
		{
			name:    "other response skips the command",
			stdin:   strings.NewReader("a\n"),
			tty:     strings.NewReader("no\n"),
			args:    []string{"-p", "echo"},
			exec:    newExec(),
			wantErr: "echo a ?...\n",
		},
		{
			name:    "end of file on the terminal skips the command",
			stdin:   strings.NewReader("a\n"),
			tty:     strings.NewReader(""),
			args:    []string{"-p", "echo"},
			exec:    newExec(),
			wantErr: "echo a ?...\n",
		},
		{
			name:     "response with more content",
			stdin:    strings.NewReader("a\n"),
			tty:      strings.NewReader("yes please\n"),
			args:     []string{"-p", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
			wantErr:  "echo a ?...\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestArgFile(t *testing.T) {
	tests := []testCase{
		{
			name:     "items are read from the file",
			fsys:     fsx.MemFS{"items.txt": {Data: []byte("a b\nc\n")}},
			args:     []string{"-a", "items.txt", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b c"},
		},
		{
			name:     "the standard input is not read",
			fsys:     fsx.MemFS{"items.txt": {Data: []byte("a\n")}},
			stdin:    strings.NewReader("b\n"),
			args:     []string{"-a", "items.txt", "-n", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
		},
		{
			name:     "- reads the standard input",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"-a", "-", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
		{
			name:     "an empty file runs the utility once",
			fsys:     fsx.MemFS{"empty.txt": {Data: []byte("")}},
			args:     []string{"-a", "empty.txt", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo"},
		},
		{
			name:     "-a skips the invocation when the file is empty and -r is given",
			fsys:     fsx.MemFS{"empty.txt": {Data: []byte("")}},
			args:     []string{"-a", "empty.txt", "-r", "echo"},
			exec:     newExec(),
			wantCmds: []string{},
		},
		{
			name:     "the last -a wins",
			fsys:     fsx.MemFS{"a.txt": {Data: []byte("a\n")}, "b.txt": {Data: []byte("b\n")}},
			args:     []string{"-a", "a.txt", "-a", "b.txt", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo b"},
		},
		{
			name:     "missing file",
			fsys:     fsx.MemFS{},
			args:     []string{"-a", "missing.txt", "echo"},
			exec:     newExec(),
			wantErr:  "xargs: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "no file system available",
			args:     []string{"-a", "items.txt", "echo"},
			exec:     newExec(),
			wantErr:  "xargs: items.txt: no file system available\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestDelimiter(t *testing.T) {
	tests := []testCase{
		{
			name:     "each delimiter terminates an argument",
			stdin:    strings.NewReader("a b\nc d\n"),
			args:     []string{"-d", `\n`, "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b c d"},
		},
		{
			name:     "blanks are not delimiters",
			stdin:    strings.NewReader("a b\nc\n"),
			args:     []string{"-d", `\n`, "-n", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b", "echo c"},
		},
		{
			name:     "space delimiter",
			stdin:    strings.NewReader("a b c "),
			args:     []string{"-d", " ", "-n", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a", "echo b", "echo c"},
		},
		{
			name:     "tab escape",
			stdin:    strings.NewReader("a\tb\tc"),
			args:     []string{"-d", `\t`, "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b c"},
		},
		{
			name:     "NUL escape",
			stdin:    strings.NewReader("a\x00b\x00"),
			args:     []string{"-d", `\0`, "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
		{
			name:     "hex escape",
			stdin:    strings.NewReader("a:b"),
			args:     []string{"-d", `\x3a`, "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
		{
			name:     "octal escape",
			stdin:    strings.NewReader("a:b"),
			args:     []string{"-d", `\72`, "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
		{
			name:     "a character above the ASCII range",
			stdin:    strings.NewReader("a\xffb\xff"),
			args:     []string{"-d", `\xff`, "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
		{
			name:     "each delimiter is a delimiter",
			stdin:    strings.NewReader("a,,b"),
			args:     []string{"-d", ",", "-n", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a", "echo ", "echo b"},
		},
		{
			name:     "trailing delimiter doesn't make an empty argument",
			stdin:    strings.NewReader("a,,b,"),
			args:     []string{"-d", ",", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a  b"},
		},
		{
			name:     "trailing data without the delimiter is used",
			stdin:    strings.NewReader("a\nb"),
			args:     []string{"-d", `\n`, "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
		{
			name:     "quotes are not special",
			stdin:    strings.NewReader(`"a b",c`),
			args:     []string{"-d", ",", "-n", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{`echo "a b"`, "echo c"},
		},
		{
			name:     "no end-of-file string",
			stdin:    strings.NewReader("END\na\n"),
			args:     []string{"-d", `\n`, "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo END a"},
		},
		{
			name:     "an empty end-of-file string has no effect either",
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"-d", `\n`, "-E", "b", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b"},
		},
		{
			name:     "replacement still applies",
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"-d", `\n`, "-I", "{}", "echo", "[{}]"},
			exec:     newExec(),
			wantCmds: []string{"echo [a]", "echo [b]"},
		},
		{
			name:     "-d after -0 wins",
			stdin:    strings.NewReader("a\npartial"),
			args:     []string{"-0", "-d", `\n`, "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a partial"},
		},
		{
			name:     "-0 after -d wins",
			stdin:    strings.NewReader("a\x00partial"),
			args:     []string{"-d", `\n`, "-0", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
		},
		{
			name:     "-d NUL keeps the trailing data, unlike -0",
			stdin:    strings.NewReader("a\x00partial"),
			args:     []string{"-d", `\0`, "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a partial"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestLongOptions(t *testing.T) {
	tests := []testCase{
		{
			name:     "--null",
			stdin:    strings.NewReader("a b\x00c\x00"),
			args:     []string{"--null", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b c"},
		},
		{
			name:     "--arg-file=FILE",
			fsys:     fsx.MemFS{"items.txt": {Data: []byte("a\nb\n")}},
			args:     []string{"--arg-file=items.txt", "-n", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a", "echo b"},
		},
		{
			name:     "an empty --arg-file is a name, not the standard input",
			fsys:     fsx.MemFS{},
			args:     []string{"--arg-file=", "echo"},
			exec:     newExec(),
			wantErr:  "xargs: : invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "--delimiter takes the value in the next argument",
			stdin:    strings.NewReader("a,b,c"),
			args:     []string{"--delimiter", ",", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b c"},
		},
		{
			name:     "--replace uses {} by default",
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"--replace", "echo", "pre-{}"},
			exec:     newExec(),
			wantCmds: []string{"echo pre-a", "echo pre-b"},
		},
		{
			name:     "--replace=R",
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"--replace=X", "echo", "[X]"},
			exec:     newExec(),
			wantCmds: []string{"echo [a]", "echo [b]"},
		},
		{
			name:     "--max-args=MAX-ARGS",
			stdin:    strings.NewReader("a b c d\n"),
			args:     []string{"--max-args=2", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b", "echo c d"},
		},
		{
			name:     "--max-lines=MAX-LINES",
			stdin:    strings.NewReader("a\nb\nc\n"),
			args:     []string{"--max-lines=2", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a b", "echo c"},
		},
		{
			name:     "--max-lines defaults to one line",
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"--max-lines", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a", "echo b"},
		},
		{
			name:  "--no-run-if-empty",
			stdin: strings.NewReader(""),
			args:  []string{"--no-run-if-empty", "echo"},
			exec:  newExec(),
		},
		{
			name:     "--interactive",
			stdin:    strings.NewReader("a\n"),
			tty:      strings.NewReader("y\n"),
			args:     []string{"--interactive", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
			wantErr:  "echo a ?...\n",
		},
		{
			name:     "--exit",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"--exit", "-n", "2", "-s", "9", "echo"},
			exec:     newExec(),
			wantErr:  "xargs: command line too long\n",
			wantExit: 1,
		},
		{
			name:     "--verbose",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"--verbose", "-n", "1", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a", "echo b"},
			wantErr:  "echo a\necho b\n",
		},
		{
			name:     "--max-chars=MAX-CHARS",
			stdin:    strings.NewReader("aaaa\n"),
			args:     []string{"--max-chars=10", "echo"},
			exec:     newExec(),
			wantErr:  "xargs: argument line too long\n",
			wantExit: 1,
		},
		{
			name:     "--eof=END",
			stdin:    strings.NewReader("a\nEND\nb\n"),
			args:     []string{"--eof=END", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a"},
		},
		{
			name:     "--eof disables the end-of-file string",
			stdin:    strings.NewReader("a\n_\nb\n"),
			args:     []string{"--eof", "echo"},
			exec:     newExec(),
			wantCmds: []string{"echo a _ b"},
		},
		{
			name:     "options end at --",
			stdin:    strings.NewReader("a\x00"),
			args:     []string{"--null", "--", "-n"},
			exec:     newExec(),
			wantCmds: []string{"-n a"},
		},
		{
			name:     "unknown long option",
			stdin:    strings.NewReader(""),
			args:     []string{"--max-procs=2"},
			exec:     newExec(),
			wantErr:  "xargs: unrecognized option '--max-procs=2'\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestExitStatuses(t *testing.T) {
	tests := []testCase{
		{
			name:     "all invocations succeed",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"-n", "1", "echo"},
			exec:     newExec(0, 0),
			wantCmds: []string{"echo a", "echo b"},
		},
		{
			name:     "any failing invocation",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"-n", "1", "echo"},
			exec:     newExec(0, 1),
			wantCmds: []string{"echo a", "echo b"},
			wantExit: 123,
		},
		{
			name:     "status 255 stops the processing",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"-n", "1", "echo"},
			exec:     newExec(255),
			wantCmds: []string{"echo a"},
			wantErr:  "xargs: echo exited with status 255\n",
			wantExit: 124,
		},
		{
			name:     "status 125 stops the processing",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"-n", "1", "echo"},
			exec:     newExec(125),
			wantCmds: []string{"echo a"},
			wantErr:  "xargs: echo exited with status 125\n",
			wantExit: 125,
		},
		{
			name:     "status 125 of the last invocation",
			stdin:    strings.NewReader("a b\n"),
			args:     []string{"-n", "1", "echo"},
			exec:     newExec(0, 125),
			wantCmds: []string{"echo a", "echo b"},
			wantErr:  "xargs: echo exited with status 125\n",
			wantExit: 125,
		},
		{
			name:     "utility not found",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"nosuchutility"},
			exec:     &execRecorder{err: errors.New("not found"), errorStatus: 127},
			wantCmds: []string{"nosuchutility a"},
			wantErr:  "xargs: nosuchutility: not found\n",
			wantExit: 127,
		},
		{
			name:     "utility cannot be run",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"nosuchutility"},
			exec:     &execRecorder{err: errors.New("permission denied")},
			wantCmds: []string{"nosuchutility a"},
			wantErr:  "xargs: nosuchutility: permission denied\n",
			wantExit: 126,
		},
		{
			name:     "utility terminated by a signal",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"echo"},
			exec:     &execRecorder{err: errors.New("terminated by signal"), errorStatus: 125},
			wantCmds: []string{"echo a"},
			wantErr:  "xargs: echo: terminated by signal\n",
			wantExit: 125,
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestInvalidUsage(t *testing.T) {
	tests := []testCase{
		{
			name:     "invalid option",
			stdin:    strings.NewReader(""),
			args:     []string{"-z"},
			exec:     newExec(),
			wantErr:  "xargs: invalid option -- 'z'\n",
			wantExit: 2,
		},
		{
			name:     "missing option argument",
			stdin:    strings.NewReader(""),
			args:     []string{"-n"},
			exec:     newExec(),
			wantErr:  "xargs: option requires an argument -- 'n'\n",
			wantExit: 2,
		},
		{
			name:     "zero count",
			stdin:    strings.NewReader(""),
			args:     []string{"-n", "0"},
			exec:     newExec(),
			wantErr:  "xargs: value for -n option should be a positive integer\n",
			wantExit: 2,
		},
		{
			name:     "non-numeric count",
			stdin:    strings.NewReader(""),
			args:     []string{"-L", "x"},
			exec:     newExec(),
			wantErr:  "xargs: value for -L option should be a positive integer\n",
			wantExit: 2,
		},
		{
			name:     "signed count",
			stdin:    strings.NewReader(""),
			args:     []string{"-n", "+1"},
			exec:     newExec(),
			wantErr:  "xargs: value for -n option should be a positive integer\n",
			wantExit: 2,
		},
		{
			name:     "signed lines count",
			stdin:    strings.NewReader(""),
			args:     []string{"-L", "+1"},
			exec:     newExec(),
			wantErr:  "xargs: value for -L option should be a positive integer\n",
			wantExit: 2,
		},
		{
			name:     "signed size",
			stdin:    strings.NewReader(""),
			args:     []string{"-s", "+1"},
			exec:     newExec(),
			wantErr:  "xargs: value for -s option should be a positive integer\n",
			wantExit: 2,
		},
		{
			name:     "zero size",
			stdin:    strings.NewReader(""),
			args:     []string{"-s", "0"},
			exec:     newExec(),
			wantErr:  "xargs: value for -s option should be a positive integer\n",
			wantExit: 2,
		},
		{
			name:     "empty replacement string",
			stdin:    strings.NewReader(""),
			args:     []string{"-I", ""},
			exec:     newExec(),
			wantErr:  "xargs: value for -I option should not be empty\n",
			wantExit: 2,
		},
		{
			name:     "empty delimiter",
			stdin:    strings.NewReader(""),
			args:     []string{"-d", ""},
			exec:     newExec(),
			wantErr:  "xargs: value for -d option should be a single character or an escape sequence\n",
			wantExit: 2,
		},
		{
			name:     "multi-character delimiter",
			stdin:    strings.NewReader(""),
			args:     []string{"-d", "xy"},
			exec:     newExec(),
			wantErr:  "xargs: value for -d option should be a single character or an escape sequence\n",
			wantExit: 2,
		},
		{
			name:     "invalid escape delimiter",
			stdin:    strings.NewReader(""),
			args:     []string{"-d", `\z`},
			exec:     newExec(),
			wantErr:  "xargs: value for -d option should be a single character or an escape sequence\n",
			wantExit: 2,
		},
		{
			name:     "delimiter escape too large",
			stdin:    strings.NewReader(""),
			args:     []string{"-d", `\x100`},
			exec:     newExec(),
			wantErr:  "xargs: value for -d option should be a single character or an escape sequence\n",
			wantExit: 2,
		},
		{
			name:     "missing argument of a long option",
			stdin:    strings.NewReader(""),
			args:     []string{"--max-args"},
			exec:     newExec(),
			wantErr:  "xargs: option '--max-args' requires an argument\n",
			wantExit: 2,
		},
		{
			name:     "unknown long option",
			stdin:    strings.NewReader(""),
			args:     []string{"--bogus"},
			exec:     newExec(),
			wantErr:  "xargs: unrecognized option '--bogus'\n",
			wantExit: 2,
		},
		{
			name:     "argument to a long option without one",
			stdin:    strings.NewReader(""),
			args:     []string{"--verbose=x"},
			exec:     newExec(),
			wantErr:  "xargs: option '--verbose=x' doesn't allow an argument\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestRunErrors(t *testing.T) {
	t.Run("no command execution available", func(t *testing.T) {
		t.Parallel()
		p := New(&coreutils.Runtime{
			Stdin:  strings.NewReader("a\n"),
			Stderr: io.Discard,
		})
		err := p.Run(t.Context())
		if code := exitCode(t, err); code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
	})

	t.Run("standard input read error", func(t *testing.T) {
		t.Parallel()
		p := New(&coreutils.Runtime{
			Stdin:  errorReader{err: errors.New("read failure")},
			Stderr: io.Discard,
			Exec:   newExec().exec,
		})
		err := p.Run(t.Context())
		var exitErr *coreutils.ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 1 {
			t.Errorf("Run() error = %v, want exit status 1", err)
		}
	})

	t.Run("canceled context stops the invocations", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		p := New(&coreutils.Runtime{
			Stdin:  strings.NewReader("a b\n"),
			Stderr: io.Discard,
			Exec: func(context.Context, string, []string, []string) (int, error) {
				cancel()
				return 125, errors.New("terminated by signal")
			},
		})
		if err := p.Run(ctx, "-n", "1", "echo"); !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})

	t.Run("canceled context stops the reading", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		p := New(&coreutils.Runtime{
			Stdin:  strings.NewReader("a\n"),
			Stderr: io.Discard,
			Exec:   newExec().exec,
		})
		if err := p.Run(ctx, "-r", "echo"); !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})

	t.Run("canceled context while prompting", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		p := New(&coreutils.Runtime{
			Stdin:  strings.NewReader("a\n"),
			Stderr: io.Discard,
			Exec:   newExec().exec,
		}).(*Prog)
		p.tty = &cancelingTty{cancel: cancel, response: 'y'}
		if err := p.Run(ctx, "-p", "echo"); !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})

	t.Run("canceled context while prompting a negative response", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		p := New(&coreutils.Runtime{
			Stdin:  strings.NewReader("a\n"),
			Stderr: io.Discard,
			Exec:   newExec().exec,
		}).(*Prog)
		p.tty = &cancelingTty{cancel: cancel, response: 'n'}
		if err := p.Run(ctx, "-p", "echo"); !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})

	t.Run("no standard error available", func(t *testing.T) {
		t.Parallel()
		p := New(&coreutils.Runtime{
			Stdin: strings.NewReader(""),
			Exec:  newExec().exec,
		})
		if code := exitCode(t, p.Run(t.Context(), "-z")); code != 2 {
			t.Errorf("exit code = %d, want 2", code)
		}
	})
}

// cancelingTty reads a single response while canceling the context.
type cancelingTty struct {
	cancel   context.CancelFunc
	response byte
	read     bool
}

func (r *cancelingTty) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	r.cancel()
	p[0] = r.response
	return 1, nil
}

func TestParseContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	data := []byte(strings.Repeat("a", 3*checkEvery))

	t.Run("tokenize", func(t *testing.T) {
		t.Parallel()
		if _, err := tokenize(ctx, data, ""); !errors.Is(err, context.Canceled) {
			t.Errorf("tokenize() error = %v, want context.Canceled", err)
		}
	})

	t.Run("splitLines", func(t *testing.T) {
		t.Parallel()
		if _, err := splitLines(ctx, data, ""); !errors.Is(err, context.Canceled) {
			t.Errorf("splitLines() error = %v, want context.Canceled", err)
		}
	})

	t.Run("splitLogicalLines", func(t *testing.T) {
		t.Parallel()
		if _, err := splitLogicalLines(ctx, data, ""); !errors.Is(err, context.Canceled) {
			t.Errorf("splitLogicalLines() error = %v, want context.Canceled", err)
		}
	})

	t.Run("splitNull", func(t *testing.T) {
		t.Parallel()
		nulls := bytes.Repeat([]byte{0}, 3*checkEvery)
		if _, err := splitNull(ctx, nulls); !errors.Is(err, context.Canceled) {
			t.Errorf("splitNull() error = %v, want context.Canceled", err)
		}
	})

	t.Run("trimTrailingBlanks", func(t *testing.T) {
		t.Parallel()
		blanks := append([]byte("a"), bytes.Repeat([]byte(" "), 3*checkEvery)...)
		if _, _, err := trimTrailingBlanks(ctx, blanks); !errors.Is(err, context.Canceled) {
			t.Errorf("trimTrailingBlanks() error = %v, want context.Canceled", err)
		}
	})

	t.Run("trimLeadingBlanks", func(t *testing.T) {
		t.Parallel()
		blanks := append(bytes.Repeat([]byte(" "), 3*checkEvery), []byte("a")...)
		if _, err := trimLeadingBlanks(ctx, blanks); !errors.Is(err, context.Canceled) {
			t.Errorf("trimLeadingBlanks() error = %v, want context.Canceled", err)
		}
	})

	t.Run("splitLogicalLines continuation", func(t *testing.T) {
		t.Parallel()
		parent, cancel := context.WithCancel(t.Context())
		ctx := &cancelAfter{Context: parent, cancel: cancel}
		data := bytes.NewBufferString("a          \n")
		for range 500 {
			data.WriteString("b          \n")
		}
		if _, err := splitLogicalLines(ctx, data.Bytes(), ""); !errors.Is(err, context.Canceled) {
			t.Errorf("splitLogicalLines() error = %v, want context.Canceled", err)
		}
	})
}

// cancelAfter cancels itself on the second Err call, so tests can reach the
// context checks deeper in the parsing loops.
type cancelAfter struct {
	context.Context
	cancel context.CancelFunc
	calls  int
}

func (c *cancelAfter) Err() error {
	c.calls++
	if c.calls == 2 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "xargs (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
