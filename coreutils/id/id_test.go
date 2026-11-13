package id

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
)

// sample is the identity the tests stub in place of the host's.
var sample = idInfo{
	user:  namedID{id: "1000", name: "alice"},
	group: namedID{id: "1000", name: "alice"},
	groups: []namedID{
		{id: "1000", name: "alice"},
		{id: "27", name: "sudo"},
	},
}

// nameless is the sample without any name resolving, as when the user
// database cannot map the IDs.
var nameless = idInfo{
	user:  namedID{id: "1000"},
	group: namedID{id: "20"},
	groups: []namedID{
		{id: "20"},
		{id: "27"},
	},
}

// noGroups is the sample without supplementary groups.
var noGroups = idInfo{
	user:  namedID{id: "1000", name: "alice"},
	group: namedID{id: "1000", name: "alice"},
}

// lines of the samples, as the utility writes them.
const (
	sampleLine   = "uid=1000(alice) gid=1000(alice) groups=1000(alice),27(sudo)\n"
	namelessLine = "uid=1000 gid=20 groups=20,27\n"
	noGroupsLine = "uid=1000(alice) gid=1000(alice)\n"
)

// errIdentity is the failure the tests stub in place of the host's.
var errIdentity = errors.New("boom")

// lookedUp is the identity the lookup hook resolves an operand to.
var lookedUp = idInfo{
	user:  namedID{id: "1001", name: "bob"},
	group: namedID{id: "1002", name: "devs"},
}

// lookupOK is a lookup hook resolving every operand.
func lookupOK(string) (idInfo, bool) {
	return lookedUp, true
}

// lookupFail is a lookup hook resolving no operand at all.
func lookupFail(string) (idInfo, bool) {
	return idInfo{}, false
}

// errWriter fails every write with its error.
type errWriter struct {
	err error
}

func (w errWriter) Write([]byte) (int, error) {
	return 0, w.err
}

// failing is an identity function which always fails.
func failing() (idInfo, error) {
	return idInfo{}, errIdentity
}

// fixed is an identity function returning the given identity.
func fixed(in idInfo) func() (idInfo, error) {
	return func() (idInfo, error) { return in, nil }
}

// run executes the utility with the given identity function and
// arguments, returning the exit code and the outputs. A nil current uses
// the identity of the host process.
func run(t *testing.T, current func() (idInfo, error), args ...string) (code int, stdout, stderr string) {
	t.Helper()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	p := New(&coreutils.Runtime{Stdout: out, Stderr: errOut}).(*Prog)
	p.current = current
	if err := p.Run(t.Context(), args...); err != nil {
		var exitErr *coreutils.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("Run() error = %v, want an ExitError", err)
		}
		code = exitErr.Code
	}
	return code, out.String(), errOut.String()
}

func TestRun(t *testing.T) {
	writeErr := errors.New("write error")

	tests := []struct {
		name     string
		id       idInfo
		identity func() (idInfo, error)
		lookup   func(string) (idInfo, bool)
		args     []string
		canceled bool
		writeErr error
		wantOut  string
		wantErr  string
		wantCode int
	}{
		{
			name:    "no options print the identity",
			id:      sample,
			wantOut: sampleLine,
		},
		{
			name:    "a double dash alone prints the identity",
			id:      sample,
			args:    []string{"--"},
			wantOut: sampleLine,
		},
		{
			name:    "unresolved names are omitted",
			id:      nameless,
			wantOut: namelessLine,
		},
		{
			name:    "groups are omitted without supplementary ones",
			id:      noGroups,
			wantOut: noGroupsLine,
		},
		{
			name:    "-G prints the group IDs",
			id:      sample,
			args:    []string{"-G"},
			wantOut: "1000 27\n",
		},
		{
			name:    "-Gn prints the group names",
			id:      sample,
			args:    []string{"-Gn"},
			wantOut: "alice sudo\n",
		},
		{
			name:    "-Gn prints the IDs which don't resolve",
			id:      nameless,
			args:    []string{"-Gn"},
			wantOut: "20 27\n",
		},
		{
			name:    "-Gz delimits the groups with NULs",
			id:      sample,
			args:    []string{"-Gz"},
			wantOut: "1000\x0027\x00",
		},
		{
			name:    "-Gnz delimits the names with NULs",
			id:      sample,
			args:    []string{"-Gnz"},
			wantOut: "alice\x00sudo\x00",
		},
		{
			name:    "-g prints the group ID",
			id:      sample,
			args:    []string{"-g"},
			wantOut: "1000\n",
		},
		{
			name:    "-gn prints the group name",
			id:      sample,
			args:    []string{"-gn"},
			wantOut: "alice\n",
		},
		{
			name:    "-gn prints the ID which doesn't resolve",
			id:      nameless,
			args:    []string{"-gn"},
			wantOut: "20\n",
		},
		{
			name:    "-u prints the user ID",
			id:      sample,
			args:    []string{"-u"},
			wantOut: "1000\n",
		},
		{
			name:    "-un prints the user name",
			id:      sample,
			args:    []string{"-un"},
			wantOut: "alice\n",
		},
		{
			name:    "-uz terminates with a NUL",
			id:      sample,
			args:    []string{"-uz"},
			wantOut: "1000\x00",
		},
		{
			name:    "-r has no effect on -u",
			id:      sample,
			args:    []string{"-ur"},
			wantOut: "1000\n",
		},
		{
			name:    "-r has no effect on -gn",
			id:      sample,
			args:    []string{"-grn"},
			wantOut: "alice\n",
		},
		{
			name:    "--groups with --name works like -Gn",
			id:      sample,
			args:    []string{"--groups", "--name"},
			wantOut: "alice sudo\n",
		},
		{
			name:    "a user operand prints its uid and gid",
			lookup:  lookupOK,
			args:    []string{"bob"},
			wantOut: "uid=1001(bob) gid=1002(devs)\n",
		},
		{
			name:    "a numeric operand resolves the same way",
			lookup:  lookupOK,
			args:    []string{"1001"},
			wantOut: "uid=1001(bob) gid=1002(devs)\n",
		},
		{
			name:    "-u with a user operand prints its uid",
			lookup:  lookupOK,
			args:    []string{"-u", "bob"},
			wantOut: "1001\n",
		},
		{
			name:    "-un with a user operand prints its name",
			lookup:  lookupOK,
			args:    []string{"-un", "bob"},
			wantOut: "bob\n",
		},
		{
			name:    "-gn with a user operand prints its group",
			lookup:  lookupOK,
			args:    []string{"-gn", "bob"},
			wantOut: "devs\n",
		},
		{
			name:    "-G with a user operand prints only the primary group",
			lookup:  lookupOK,
			args:    []string{"-G", "bob"},
			wantOut: "1002\n",
		},
		{
			name:    "-Gz with a user operand terminates with a NUL",
			lookup:  lookupOK,
			args:    []string{"-Gz", "bob"},
			wantOut: "1002\x00",
		},
		{
			name:    "-r with a user operand is without effect",
			lookup:  lookupOK,
			args:    []string{"-ur", "bob"},
			wantOut: "1001\n",
		},
		{
			name:     "rejects an unknown user",
			lookup:   lookupFail,
			args:     []string{"bob"},
			wantErr:  "id: 'bob': no such user\n",
			wantCode: 1,
		},
		{
			name:    "an empty operand is looked up like any other",
			lookup:  lookupOK,
			args:    []string{""},
			wantOut: "uid=1001(bob) gid=1002(devs)\n",
		},
		{
			name:     "an empty operand resolves no user",
			lookup:   lookupFail,
			args:     []string{""},
			wantErr:  "id: '': no such user\n",
			wantCode: 1,
		},
		{
			name:     "reports the conflicting options before the extra operand",
			lookup:   lookupOK,
			args:     []string{"-u", "-g", "bob", "alice"},
			wantErr:  "id: cannot print \"only\" of more than one choice\nTry 'id --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "rejects a second operand",
			lookup:   lookupOK,
			args:     []string{"bob", "alice"},
			wantErr:  "id: extra operand 'alice'\nTry 'id --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "rejects -u with -g",
			id:       sample,
			args:     []string{"-u", "-g"},
			wantErr:  "id: cannot print \"only\" of more than one choice\nTry 'id --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "rejects -g with -G",
			id:       sample,
			args:     []string{"-g", "--groups"},
			wantErr:  "id: cannot print \"only\" of more than one choice\nTry 'id --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "rejects -n without -u, -g or -G",
			id:       sample,
			args:     []string{"-n"},
			wantErr:  "id: printing only names or real IDs requires -u, -g, or -G\nTry 'id --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "rejects -r without -u, -g or -G",
			id:       sample,
			args:     []string{"-r"},
			wantErr:  "id: printing only names or real IDs requires -u, -g, or -G\nTry 'id --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "rejects -z in the default format",
			id:       sample,
			args:     []string{"-z"},
			wantErr:  "id: option --zero not permitted in default format\nTry 'id --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "rejects -n with -z before the zero diagnostic",
			id:       sample,
			args:     []string{"-nz"},
			wantErr:  "id: printing only names or real IDs requires -u, -g, or -G\nTry 'id --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "rejects an unknown option",
			id:       sample,
			args:     []string{"-x"},
			wantErr:  "id: invalid option -- 'x'\nTry 'id --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "rejects an unknown long option",
			id:       sample,
			args:     []string{"--bogus"},
			wantErr:  "id: unrecognized option '--bogus'\nTry 'id --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "treats options after a double dash as operands",
			lookup:   lookupFail,
			args:     []string{"--", "-u"},
			wantErr:  "id: '-u': no such user\n",
			wantCode: 1,
		},
		{
			name:     "fails when the identity cannot be read",
			identity: failing,
			wantErr:  "id: boom\n",
			wantCode: 1,
		},
		{
			name:     "reports a write error",
			id:       sample,
			writeErr: writeErr,
			wantCode: 1,
		},
		{
			name:     "stops on a canceled context",
			id:       sample,
			canceled: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			if test.canceled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			stdout := new(bytes.Buffer)
			var out io.Writer = stdout
			if test.writeErr != nil {
				out = errWriter{err: test.writeErr}
			}
			stderr := new(bytes.Buffer)
			p := New(&coreutils.Runtime{Stdout: out, Stderr: stderr}).(*Prog)
			p.current = test.identity
			p.lookup = test.lookup
			if p.current == nil && p.lookup == nil {
				p.current = fixed(test.id)
			}
			err := p.Run(ctx, test.args...)

			if test.canceled {
				if !errors.Is(err, context.Canceled) {
					t.Errorf("Run() error = %v, want context.Canceled", err)
				}
				if got := stdout.String(); got != "" {
					t.Errorf("Run() stdout = %q, want empty", got)
				}
				return
			}

			if test.wantCode == 0 {
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
			} else {
				var exitErr *coreutils.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("Run() error = %v, want an ExitError", err)
				}
				if exitErr.Code != test.wantCode {
					t.Errorf("Run() exit code = %d, want %d", exitErr.Code, test.wantCode)
				}
			}
			if test.writeErr != nil {
				if !errors.Is(err, test.writeErr) {
					t.Errorf("Run() error = %v, want it to wrap the write error", err)
				}
				return
			}
			if got := stdout.String(); got != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", got, test.wantOut)
			}
			if got := stderr.String(); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
		})
	}
}

func TestFormatting(t *testing.T) {
	t.Run("line", func(t *testing.T) {
		tests := []struct {
			name string
			in   idInfo
			want string
		}{
			{
				name: "names and groups",
				in:   sample,
				want: "uid=1000(alice) gid=1000(alice) groups=1000(alice),27(sudo)",
			},
			{
				name: "names which don't resolve",
				in:   nameless,
				want: "uid=1000 gid=20 groups=20,27",
			},
			{
				name: "mixed resolution",
				in: idInfo{
					user:  namedID{id: "1", name: "daemon"},
					group: namedID{id: "1"},
					groups: []namedID{
						{id: "1", name: "daemon"},
						{id: "2"},
					},
				},
				want: "uid=1(daemon) gid=1 groups=1(daemon),2",
			},
			{
				name: "without groups",
				in:   noGroups,
				want: "uid=1000(alice) gid=1000(alice)",
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if got := test.in.line(); got != test.want {
					t.Errorf("line() = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("list", func(t *testing.T) {
		tests := []struct {
			name string
			in   idInfo
			want []namedID
		}{
			{
				name: "primary first, then the supplementary",
				in:   sample,
				want: []namedID{{"1000", "alice"}, {"27", "sudo"}},
			},
			{
				name: "skips repeats of the primary",
				in: idInfo{
					user:  namedID{id: "1000", name: "alice"},
					group: namedID{id: "20", name: "staff"},
					groups: []namedID{
						{id: "20", name: "staff"},
						{id: "20", name: "staff"},
						{id: "27", name: "sudo"},
					},
				},
				want: []namedID{{"20", "staff"}, {"27", "sudo"}},
			},
			{
				name: "without supplementary groups",
				in:   noGroups,
				want: []namedID{{"1000", "alice"}},
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				var got []string
				for _, g := range test.in.list() {
					got = append(got, g.id)
				}
				var want []string
				for _, g := range test.want {
					want = append(want, g.id)
				}
				if strings.Join(got, ",") != strings.Join(want, ",") {
					t.Errorf("list() = %v, want %v", got, want)
				}
			})
		}
	})

	t.Run("value", func(t *testing.T) {
		tests := []struct {
			name  string
			id    namedID
			names bool
			want  string
		}{
			{"ID without names", namedID{"1000", "alice"}, false, "1000"},
			{"name with names", namedID{"1000", "alice"}, true, "alice"},
			{"ID when the name is empty", namedID{"1000", ""}, true, "1000"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if got := test.id.value(test.names); got != test.want {
					t.Errorf("value(%v) = %q, want %q", test.names, got, test.want)
				}
			})
		}
	})

	t.Run("tagged", func(t *testing.T) {
		tests := []struct {
			name string
			id   namedID
			want string
		}{
			{"ID with a name", namedID{"1000", "alice"}, "1000(alice)"},
			{"ID without a name", namedID{"1000", ""}, "1000"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if got := test.id.tagged(); got != test.want {
					t.Errorf("tagged() = %q, want %q", got, test.want)
				}
			})
		}
	})
}

func TestHelpVersion(t *testing.T) {
	for _, opt := range []string{"--help", "--version"} {
		t.Run(opt, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
			if err := New(rt).Run(t.Context(), opt); err != nil {
				t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
			}
			want := fmt.Sprintf(
				"id (charm.land/turtle/coreutils) %s\nhttps://charm.land/turtle\n",
				coreutils.Version)
			if opt == "--help" {
				want = usage
			}
			if got := stdout.String(); got != want {
				t.Errorf("Run() stdout = %q, want %q", got, want)
			}
			if got := stderr.String(); got != "" {
				t.Errorf("Run() stderr = %q, want empty", got)
			}
		})
	}
}

func TestHostIdentity(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current() error = %v", err)
	}
	in, err := hostIdentity()
	if err != nil {
		t.Fatalf("hostIdentity() error = %v", err)
	}
	if in.user.id != u.Uid {
		t.Errorf("user id = %q, want %q", in.user.id, u.Uid)
	}
	if in.user.name != u.Username {
		t.Errorf("user name = %q, want %q", in.user.name, u.Username)
	}
	if in.group.id != u.Gid {
		t.Errorf("group id = %q, want %q", in.group.id, u.Gid)
	}

	if runtime.GOOS == "windows" {
		// Windows reports the SID strings instead of numeric IDs, and no
		// supplementary groups.
		if !strings.HasPrefix(in.user.id, "S-1-") || !strings.HasPrefix(in.group.id, "S-1-") {
			t.Errorf("ids = %q, %q, want SID strings", in.user.id, in.group.id)
		}
		if in.groups != nil {
			t.Errorf("groups = %v, want none", in.groups)
		}
		return
	}

	if in.user.id != strconv.Itoa(os.Getuid()) {
		t.Errorf("user id = %q, want %d", in.user.id, os.Getuid())
	}
	if in.group.id != strconv.Itoa(os.Getgid()) {
		t.Errorf("group id = %q, want %d", in.group.id, os.Getgid())
	}
	gids, err := os.Getgroups()
	if err != nil {
		t.Fatalf("os.Getgroups() error = %v", err)
	}
	var want []string
	for _, gid := range gids {
		want = append(want, strconv.Itoa(gid))
	}
	var got []string
	for _, g := range in.groups {
		got = append(got, g.id)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("groups = %v, want %v", got, want)
	}
}

func TestRunHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports no numeric IDs; see TestRunHostWindows")
	}

	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current() error = %v", err)
	}

	tests := []struct {
		name    string
		args    []string
		wantOut string
	}{
		{
			name:    "-u prints the effective user ID",
			args:    []string{"-u"},
			wantOut: strconv.Itoa(os.Getuid()) + "\n",
		},
		{
			name:    "-un prints the user name",
			args:    []string{"-un"},
			wantOut: u.Username + "\n",
		},
		{
			name:    "-g prints the effective group ID",
			args:    []string{"-g"},
			wantOut: strconv.Itoa(os.Getgid()) + "\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, out, errOut := run(t, nil, test.args...)
			if code != 0 {
				t.Errorf("Run() exit code = %d, want 0", code)
			}
			if out != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", out, test.wantOut)
			}
			if errOut != "" {
				t.Errorf("Run() stderr = %q, want empty", errOut)
			}
		})
	}

	t.Run("a user operand resolves through the database", func(t *testing.T) {
		// Some environments, like bare containers, hold no entry for the
		// current user in the database.
		if _, ok := hostLookup(u.Username); !ok {
			t.Skip("the host database does not resolve the current user")
		}
		code, out, errOut := run(t, nil, "-u", u.Username)
		if code != 0 {
			t.Errorf("Run() exit code = %d, want 0", code)
		}
		if out != u.Uid+"\n" {
			t.Errorf("Run() stdout = %q, want %q", out, u.Uid+"\n")
		}
		if errOut != "" {
			t.Errorf("Run() stderr = %q, want empty", errOut)
		}
	})

	t.Run("a numeric operand resolves as a uid", func(t *testing.T) {
		if _, ok := hostLookup(u.Uid); !ok {
			t.Skip("the host database does not resolve the current user")
		}
		code, out, errOut := run(t, nil, "-un", u.Uid)
		if code != 0 {
			t.Errorf("Run() exit code = %d, want 0", code)
		}
		if out != u.Username+"\n" {
			t.Errorf("Run() stdout = %q, want %q", out, u.Username+"\n")
		}
		if errOut != "" {
			t.Errorf("Run() stderr = %q, want empty", errOut)
		}
	})

	t.Run("a user operand starts with its identity", func(t *testing.T) {
		if _, ok := hostLookup(u.Username); !ok {
			t.Skip("the host database does not resolve the current user")
		}
		code, out, errOut := run(t, nil, u.Username)
		if code != 0 {
			t.Errorf("Run() exit code = %d, want 0", code)
		}
		want := fmt.Sprintf("uid=%s(%s) gid=", u.Uid, u.Username)
		if !strings.HasPrefix(out, want) {
			t.Errorf("Run() stdout = %q, want it to start with %q", out, want)
		}
		if strings.Contains(out, " groups=") {
			t.Errorf("Run() stdout = %q, want no groups for a user operand", out)
		}
		if errOut != "" {
			t.Errorf("Run() stderr = %q, want empty", errOut)
		}
	})

	t.Run("an unknown user operand fails", func(t *testing.T) {
		code, out, errOut := run(t, nil, "no-such-user-xyzzy-4096")
		if code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
		if out != "" {
			t.Errorf("Run() stdout = %q, want empty", out)
		}
		want := "id: 'no-such-user-xyzzy-4096': no such user\n"
		if errOut != want {
			t.Errorf("Run() stderr = %q, want %q", errOut, want)
		}
	})

	t.Run("default format starts with the identity", func(t *testing.T) {
		code, out, errOut := run(t, nil)
		if code != 0 {
			t.Errorf("Run() exit code = %d, want 0", code)
		}
		want := fmt.Sprintf("uid=%s(%s) gid=", u.Uid, u.Username)
		if !strings.HasPrefix(out, want) {
			t.Errorf("Run() stdout = %q, want it to start with %q", out, want)
		}
		if errOut != "" {
			t.Errorf("Run() stderr = %q, want empty", errOut)
		}
	})
}

func TestRunHostWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Unix reports numeric IDs; see TestRunHost")
	}

	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current() error = %v", err)
	}

	t.Run("default format reports the SIDs", func(t *testing.T) {
		code, out, errOut := run(t, nil)
		if code != 0 {
			t.Errorf("Run() exit code = %d, want 0", code)
		}
		want := fmt.Sprintf("uid=%s(%s) gid=%s", u.Uid, u.Username, u.Gid)
		if !strings.HasPrefix(out, want) {
			t.Errorf("Run() stdout = %q, want it to start with %q", out, want)
		}
		if strings.Contains(out, " groups=") {
			t.Errorf("Run() stdout = %q, want no supplementary groups", out)
		}
		if errOut != "" {
			t.Errorf("Run() stderr = %q, want empty", errOut)
		}
	})

	t.Run("-un prints the user name", func(t *testing.T) {
		_, out, _ := run(t, nil, "-un")
		if out != u.Username+"\n" {
			t.Errorf("Run() stdout = %q, want %q", out, u.Username+"\n")
		}
	})

	t.Run("a user operand resolves through the database", func(t *testing.T) {
		if _, ok := hostLookup(u.Username); !ok {
			t.Skip("the host database does not resolve the current user")
		}
		_, out, _ := run(t, nil, "-u", u.Username)
		if out != u.Uid+"\n" {
			t.Errorf("Run() stdout = %q, want %q", out, u.Uid+"\n")
		}
	})
}
