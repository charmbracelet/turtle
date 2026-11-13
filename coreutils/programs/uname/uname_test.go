package uname

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
)

// sample is the system information the tests stub in place of the host's.
var sample = fields{
	sysname:          "TurtleOS",
	nodename:         "shell.example.org",
	release:          "1.2.3-turtle",
	version:          "#1 SMP Turtle Jul 5 10:00:00 UTC 2026",
	machine:          "turtle64",
	processor:        "turtle-isa",
	hardwarePlatform: "turtle-platform",
	osName:           "GNU/Turtle",
}

// unknownProcessor is the sample without the processor type and the
// hardware platform, which most systems don't report.
var unknownProcessor = func() fields {
	f := sample
	f.processor, f.hardwarePlatform = "", ""
	return f
}()

// emptyNodename is the sample without a node name.
var emptyNodename = func() fields {
	f := sample
	f.nodename = ""
	return f
}()

// errWriter fails every write with its error.
type errWriter struct {
	err error
}

func (w errWriter) Write([]byte) (int, error) {
	return 0, w.err
}

// run executes the utility with the given stubbed fields and arguments,
// returning the exit code and the outputs.
func run(t *testing.T, f fields, collectErr error, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	p := New(&coreutils.Runtime{Stdout: out, Stderr: errOut}).(*Prog)
	p.collect = func() (fields, error) { return f, collectErr }
	if err := p.Run(t.Context(), args...); err != nil {
		var exitErr *coreutils.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("Run() error = %v, want an ExitError", err)
		}
		code = exitErr.Code
	}
	return code, out.String(), errOut.String()
}

// line joins the sample pieces the way -a writes them.
const (
	allLine   = "TurtleOS shell.example.org 1.2.3-turtle #1 SMP Turtle Jul 5 10:00:00 UTC 2026 turtle64 turtle-isa turtle-platform GNU/Turtle\n"
	posixLine = "TurtleOS shell.example.org 1.2.3-turtle #1 SMP Turtle Jul 5 10:00:00 UTC 2026 turtle64\n"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name   string
		fields *fields
		args   []string
		code   int
		out    string
		err    string
	}{
		{
			name: "no options print the system name",
			out:  "TurtleOS\n",
		},
		{
			name: "a double dash alone prints the system name",
			args: []string{"--"},
			out:  "TurtleOS\n",
		},
		{
			name: "-s prints the system name",
			args: []string{"-s"},
			out:  "TurtleOS\n",
		},
		{
			name: "-n prints the node name",
			args: []string{"-n"},
			out:  "shell.example.org\n",
		},
		{
			name: "-r prints the release",
			args: []string{"-r"},
			out:  "1.2.3-turtle\n",
		},
		{
			name: "-v prints the version",
			args: []string{"-v"},
			out:  "#1 SMP Turtle Jul 5 10:00:00 UTC 2026\n",
		},
		{
			name: "-m prints the machine",
			args: []string{"-m"},
			out:  "turtle64\n",
		},
		{
			name: "options after a double dash are operands",
			args: []string{"--", "-s"},
			code: 1,
			err:  "uname: extra operand '-s'\nTry 'uname --help' for more information.\n",
		},
		{
			name: "a double dash ends the options",
			args: []string{"-s", "--"},
			out:  "TurtleOS\n",
		},
		{
			name: "grouped options keep the specified order",
			args: []string{"-nr"},
			out:  "shell.example.org 1.2.3-turtle\n",
		},
		{
			name: "the arguments order doesn't change the output order",
			args: []string{"-rn"},
			out:  "shell.example.org 1.2.3-turtle\n",
		},
		{
			name: "grouped options with the version and machine",
			args: []string{"-vm"},
			out:  "#1 SMP Turtle Jul 5 10:00:00 UTC 2026 turtle64\n",
		},
		{
			name: "repeated options print their information once",
			args: []string{"-mnm"},
			out:  "shell.example.org turtle64\n",
		},
		{
			name: "the POSIX set of options",
			args: []string{"-snrvm"},
			out:  posixLine,
		},
		{
			name: "-s with a subset of options",
			args: []string{"-s", "-r", "-m"},
			out:  "TurtleOS 1.2.3-turtle turtle64\n",
		},
		{
			name: "-a prints all of the POSIX information",
			args: []string{"-a"},
			out:  allLine,
		},
		{
			name: "-a with -s still prints everything once",
			args: []string{"-a", "-s"},
			out:  allLine,
		},
		{
			name:   "-a omits the processor and hardware platform when unknown",
			fields: &unknownProcessor,
			args:   []string{"-a"},
			out:    "TurtleOS shell.example.org 1.2.3-turtle #1 SMP Turtle Jul 5 10:00:00 UTC 2026 turtle64 GNU/Turtle\n",
		},
		{
			name:   "-a omits any unavailable field",
			fields: &emptyNodename,
			args:   []string{"-a"},
			out:    "TurtleOS 1.2.3-turtle #1 SMP Turtle Jul 5 10:00:00 UTC 2026 turtle64 turtle-isa turtle-platform GNU/Turtle\n",
		},
		{
			name:   "an unavailable field prints unknown alone",
			fields: &emptyNodename,
			args:   []string{"-n"},
			out:    "unknown\n",
		},
		{
			name: "-p prints the processor type",
			args: []string{"-p"},
			out:  "turtle-isa\n",
		},
		{
			name: "-i prints the hardware platform",
			args: []string{"-i"},
			out:  "turtle-platform\n",
		},
		{
			name: "-o prints the operating system",
			args: []string{"-o"},
			out:  "GNU/Turtle\n",
		},
		{
			name: "grouped GNU options",
			args: []string{"-pi"},
			out:  "turtle-isa turtle-platform\n",
		},
		{
			name: "the GNU options follow the machine",
			args: []string{"-mpio"},
			out:  "turtle64 turtle-isa turtle-platform GNU/Turtle\n",
		},
		{
			name:   "-p prints unknown when the processor type is unavailable",
			fields: &unknownProcessor,
			args:   []string{"-p"},
			out:    "unknown\n",
		},
		{
			name:   "-i prints unknown when the platform is unavailable",
			fields: &unknownProcessor,
			args:   []string{"-i"},
			out:    "unknown\n",
		},
		{
			name:   "both unknown GNU fields print unknown",
			fields: &unknownProcessor,
			args:   []string{"-p", "-i"},
			out:    "unknown unknown\n",
		},
		{
			name: "a long option for every short one",
			args: []string{"--kernel-name", "--nodename", "--kernel-release"},
			out:  "TurtleOS shell.example.org 1.2.3-turtle\n",
		},
		{
			name: "--all",
			args: []string{"--all"},
			out:  allLine,
		},
		{
			name: "--machine",
			args: []string{"--machine"},
			out:  "turtle64\n",
		},
		{
			name: "--processor",
			args: []string{"--processor"},
			out:  "turtle-isa\n",
		},
		{
			name: "--hardware-platform",
			args: []string{"--hardware-platform"},
			out:  "turtle-platform\n",
		},
		{
			name: "--operating-system",
			args: []string{"--operating-system"},
			out:  "GNU/Turtle\n",
		},
		{
			name: "--kernel-version",
			args: []string{"--kernel-version"},
			out:  "#1 SMP Turtle Jul 5 10:00:00 UTC 2026\n",
		},
		{
			name: "an invalid option",
			args: []string{"-x"},
			code: 1,
			err:  "uname: invalid option -- 'x'\nTry 'uname --help' for more information.\n",
		},
		{
			name: "an invalid option in a group",
			args: []string{"-ax"},
			code: 1,
			err:  "uname: invalid option -- 'x'\nTry 'uname --help' for more information.\n",
		},
		{
			name: "an unrecognized long option",
			args: []string{"--bogus"},
			code: 1,
			err:  "uname: unrecognized option '--bogus'\nTry 'uname --help' for more information.\n",
		},
		{
			name: "a long option with an argument",
			args: []string{"--all=yes"},
			code: 1,
			err:  "uname: option '--all=yes' doesn't allow an argument\nTry 'uname --help' for more information.\n",
		},
		{
			name: "an operand",
			args: []string{"-s", "linux"},
			code: 1,
			err:  "uname: extra operand 'linux'\nTry 'uname --help' for more information.\n",
		},
		{
			name: "an operand before the options",
			args: []string{"linux", "-s"},
			code: 1,
			err:  "uname: extra operand 'linux'\nTry 'uname --help' for more information.\n",
		},
		{
			name: "an operand after a double dash",
			args: []string{"--", "linux"},
			code: 1,
			err:  "uname: extra operand 'linux'\nTry 'uname --help' for more information.\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := sample
			if test.fields != nil {
				f = *test.fields
			}
			code, out, errOut := run(t, f, nil, test.args...)
			if code != test.code {
				t.Errorf("Run() exit code = %d, want %d", code, test.code)
			}
			if out != test.out {
				t.Errorf("Run() stdout = %q, want %q", out, test.out)
			}
			if errOut != test.err {
				t.Errorf("Run() stderr = %q, want %q", errOut, test.err)
			}
			if test.out != "" && strings.Count(out, "\n") != 1 {
				t.Errorf("Run() stdout = %q, want a single line", out)
			}
		})
	}
}

func TestCollectError(t *testing.T) {
	collectErr := errors.New("cannot read the system name")
	code, out, errOut := run(t, fields{}, collectErr, "-s")
	if code != 1 {
		t.Errorf("Run() exit code = %d, want 1", code)
	}
	if out != "" {
		t.Errorf("Run() stdout = %q, want empty", out)
	}
	if want := "uname: cannot read the system name\n"; errOut != want {
		t.Errorf("Run() stderr = %q, want %q", errOut, want)
	}
}

func TestWriteError(t *testing.T) {
	writeErr := errors.New("write error")
	p := New(&coreutils.Runtime{
		Stdout: errWriter{err: writeErr},
		Stderr: new(bytes.Buffer),
	}).(*Prog)
	p.collect = func() (fields, error) { return sample, nil }

	err := p.Run(t.Context(), "-s")
	var exitErr *coreutils.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %v, want an ExitError", err)
	}
	if exitErr.Code != 1 {
		t.Errorf("Run() exit code = %d, want 1", exitErr.Code)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("Run() error = %v, want it to wrap the write error", err)
	}
}

func TestCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	p := New(&coreutils.Runtime{Stdout: stdout, Stderr: stderr}).(*Prog)
	p.collect = func() (fields, error) {
		t.Error("collect() called on a canceled context")
		return sample, nil
	}
	err := p.Run(ctx, "-s")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("Run() wrote %q and %q, want nothing", stdout, stderr)
	}
}

func TestHelpVersion(t *testing.T) {
	// The host information is never collected for the standard options.
	collect := func() (fields, error) {
		t.Error("collect() called for a standard option")
		return fields{}, nil
	}

	tests := []struct {
		name string
		args []string
		out  string
	}{
		{
			name: "help",
			args: []string{"--help"},
			out:  usage,
		},
		{
			name: "version",
			args: []string{"--version"},
			out: "uname (charm.land/turtle/coreutils) " + coreutils.Version +
				"\nhttps://charm.land/turtle\n",
		},
		{
			name: "help before an invalid option",
			args: []string{"--help", "-x"},
			out:  usage,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			p := New(&coreutils.Runtime{Stdout: stdout, Stderr: stderr}).(*Prog)
			p.collect = collect
			if err := p.Run(t.Context(), test.args...); err != nil {
				t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
			}
			if got := stdout.String(); got != test.out {
				t.Errorf("Run() stdout = %q, want %q", got, test.out)
			}
			if stderr.Len() != 0 {
				t.Errorf("Run() stderr = %q, want empty", stderr)
			}
		})
	}
}

func TestHostRun(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
	if err := New(rt).Run(t.Context()); err != nil {
		t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
	}
	out := stdout.String()
	if out == "" || !strings.HasSuffix(out, "\n") || strings.Count(out, "\n") != 1 {
		t.Errorf("Run() stdout = %q, want a single non-empty line", out)
	}
}

func TestHostFields(t *testing.T) {
	f, err := hostFields()
	if err != nil {
		t.Fatalf("hostFields() error = %v", err)
	}
	if f.sysname == "" {
		t.Errorf("hostFields() sysname = %q, want a system name", f.sysname)
	}
	switch runtime.GOOS {
	case "linux":
		if f.sysname != "Linux" {
			t.Errorf("hostFields() sysname = %q, want %q", f.sysname, "Linux")
		}
		if f.osName != "GNU/Linux" {
			t.Errorf("hostFields() osName = %q, want %q", f.osName, "GNU/Linux")
		}
		if f.machine == "" || f.hardwarePlatform != f.machine {
			t.Errorf("hostFields() machine = %q and hardware platform = %q, want both set",
				f.machine, f.hardwarePlatform)
		}
	case "android":
		if f.sysname != "Linux" {
			t.Errorf("hostFields() sysname = %q, want %q", f.sysname, "Linux")
		}
		if f.osName != "Android" {
			t.Errorf("hostFields() osName = %q, want %q", f.osName, "Android")
		}
	case "darwin":
		if f.sysname != "Darwin" {
			t.Errorf("hostFields() sysname = %q, want %q", f.sysname, "Darwin")
		}
		if f.release == "" || f.version == "" || f.machine == "" {
			t.Errorf("hostFields() = %+v, want the release, version and machine set", f)
		}
	case "windows":
		if f.sysname != "Windows_NT" {
			t.Errorf("hostFields() sysname = %q, want %q", f.sysname, "Windows_NT")
		}
		if want := goarchMachine(runtime.GOARCH); f.machine != want {
			t.Errorf("hostFields() machine = %q, want %q", f.machine, want)
		}
	}
}

func TestGoarchMachine(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"amd64", "x86_64"},
		{"386", "i686"},
		{"arm64", "arm64"},
		{"arm", "arm"},
		{"riscv64", "riscv64"},
		{"wasm", "wasm"},
	}
	for _, test := range tests {
		t.Run(test.arch, func(t *testing.T) {
			if got := goarchMachine(test.arch); got != test.want {
				t.Errorf("goarchMachine(%q) = %q, want %q", test.arch, got, test.want)
			}
		})
	}
}

// optionToken matches a usage option token: a short form alone, or a
// combined short and long form, as in "-a, --all".
var optionToken = regexp.MustCompile(`^(-[a-z], )?--[a-z-]+$|^-[a-z]$`)

// TestUsageLayout checks that the option table keeps the hanging layout of
// the GNU usage texts: an option token alone on its line at 2 or 6 spaces,
// with its description below at 9 spaces, all wrapped at 80 columns.
func TestUsageLayout(t *testing.T) {
	for i, line := range strings.Split(usage, "\n") {
		if len(line) > 80 {
			t.Errorf("usage line %d (%q) is %d columns long, want at most 80",
				i+1, line, len(line))
		}
		if line == "" || !strings.HasPrefix(line, " ") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		token := strings.TrimLeft(line, " ")
		switch {
		case (indent == 2 || indent == 6) && strings.HasPrefix(token, "-"):
			if !optionToken.MatchString(token) {
				t.Errorf("usage line %d (%q) is not a lone option token", i+1, line)
			}
		case indent == 9:
			// A description line.
		default:
			t.Errorf("usage line %d (%q) is neither an option token at 2 or 6 spaces nor a description at 9", i+1, line)
		}
	}
}
