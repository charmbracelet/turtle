package chown

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// The host user database the tests stub, so the names resolve without
// depending on the host.
var (
	testUsers = map[string]user.User{
		"alice": {Username: "alice", Uid: "1000", Gid: "1000"},
		"bob":   {Username: "bob", Uid: "1001", Gid: "1001"},
	}
	testGroups = map[string]user.Group{
		"staff": {Name: "staff", Gid: "1000"},
		"devs":  {Name: "devs", Gid: "1001"},
		"wheel": {Name: "wheel", Gid: "0"},
	}
	testUserIDs = map[string]user.User{
		"0":    {Username: "root", Uid: "0", Gid: "0"},
		"1000": {Username: "alice", Uid: "1000", Gid: "1000"},
		"1001": {Username: "bob", Uid: "1001", Gid: "1001"},
	}
	testGroupIDs = map[string]user.Group{
		"0":    {Name: "root", Gid: "0"},
		"1000": {Name: "staff", Gid: "1000"},
		"1001": {Name: "devs", Gid: "1001"},
	}
)

func testUser(name string) (*user.User, error) {
	if u, ok := testUsers[name]; ok {
		return &u, nil
	}
	return nil, user.UnknownUserError(name)
}

func testGroup(name string) (*user.Group, error) {
	if g, ok := testGroups[name]; ok {
		return &g, nil
	}
	return nil, user.UnknownGroupError(name)
}

func testUserID(id string) (*user.User, error) {
	if u, ok := testUserIDs[id]; ok {
		return &u, nil
	}
	return nil, errors.New("no such user")
}

func testGroupID(id string) (*user.Group, error) {
	if g, ok := testGroupIDs[id]; ok {
		return &g, nil
	}
	return nil, errors.New("no such group")
}

// file is a regular file owned by the given IDs.
func file(uid, gid int) *fsx.MemFile {
	return &fsx.MemFile{Uid: uid, Gid: gid}
}

// dir is a directory owned by the given IDs.
func dir(uid, gid int) *fsx.MemFile {
	return &fsx.MemFile{Mode: fs.ModeDir | 0o755, Uid: uid, Gid: gid}
}

// link is a symbolic link owned by the given IDs.
func link(target string, uid, gid int) *fsx.MemFile {
	return &fsx.MemFile{Data: []byte(target), Mode: fs.ModeSymlink | 0o777, Uid: uid, Gid: gid}
}

// owner is the ownership a file is expected to have.
type owner struct {
	uid int
	gid int
}

type testCase struct {
	name     string
	files    fsx.MemFS
	args     []string
	hook     func(*Prog)
	want     map[string]owner
	wantOut  string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: stdout,
		Stderr: stderr,
		FS:     test.files,
	}
	p := New(rt).(*Prog)
	p.lookupUser, p.lookupGroup = testUser, testGroup
	p.lookupUserID, p.lookupGroupID = testUserID, testGroupID
	if test.hook != nil {
		test.hook(p)
	}

	err := p.Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.wantOut {
		t.Errorf("Run() stdout = %q, want %q", got, test.wantOut)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}

	for name, want := range test.want {
		fi, err := test.files.Lstat(name)
		if err != nil {
			t.Errorf("Lstat(%q) error = %v", name, err)
			continue
		}
		uid, gid, ok := fsx.Owner(fi)
		if !ok {
			t.Errorf("Owner(%q) = unknown, want %d:%d", name, want.uid, want.gid)
			continue
		}
		if uid != want.uid || gid != want.gid {
			t.Errorf("%s ownership = %d:%d, want %d:%d", name, uid, gid, want.uid, want.gid)
		}
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

// runTests runs the given cases as subtests.
func runTests(t *testing.T, tests []testCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOwner(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "sets the owner by name",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{"bob", "file.txt"},
			want:  map[string]owner{"file.txt": {1001, 1000}},
		},
		{
			name:  "sets the owner by numeric ID",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{"1001", "file.txt"},
			want:  map[string]owner{"file.txt": {1001, 1000}},
		},
		{
			name:  "a + prefixed owner is forced numeric",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{"+1001", "file.txt"},
			want:  map[string]owner{"file.txt": {1001, 1000}},
		},
		{
			name:  "sets the owner of every file",
			files: fsx.MemFS{"a.txt": file(1000, 1000), "b.txt": file(1000, 1000)},
			args:  []string{"0", "a.txt", "b.txt"},
			want:  map[string]owner{"a.txt": {0, 1000}, "b.txt": {0, 1000}},
		},
		{
			name:  "keeps the group",
			files: fsx.MemFS{"file.txt": file(1000, 1001)},
			args:  []string{"0", "file.txt"},
			want:  map[string]owner{"file.txt": {0, 1001}},
		},
		{
			name:  "changes a directory",
			files: fsx.MemFS{"d": dir(1000, 1000)},
			args:  []string{"0", "d"},
			want:  map[string]owner{"d": {0, 1000}},
		},
		{
			name:     "an unknown user is an error",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"nosuch", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: invalid user: \"nosuch\"\n",
			wantExit: 1,
		},
		{
			name:     "a + prefixed non numeric owner is an error",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"+x", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: invalid user: \"+x\"\n",
			wantExit: 1,
		},
		{
			name:  "a numeric owner that names a user wins as the name",
			files: fsx.MemFS{"file.txt": file(0, 0)},
			args:  []string{"1000", "file.txt"},
			hook:  numericNameUser,
			want:  map[string]owner{"file.txt": {500, 0}},
		},
		{
			name:  "+ forces the numeric interpretation",
			files: fsx.MemFS{"file.txt": file(0, 0)},
			args:  []string{"+1000", "file.txt"},
			hook:  numericNameUser,
			want:  map[string]owner{"file.txt": {1000, 0}},
		},
	})
}

// numericNameUser replaces the lookup with a user literally named "1000"
// whose ID is 500, which the utility must prefer as a name.
func numericNameUser(p *Prog) {
	named := &user.User{Username: "1000", Uid: "500", Gid: "500"}
	p.lookupUser = func(name string) (*user.User, error) {
		if name == "1000" {
			return named, nil
		}
		return testUser(name)
	}
	p.lookupUserID = func(id string) (*user.User, error) {
		if id == "500" {
			return named, nil
		}
		return testUserID(id)
	}
}

// numericNameGroup replaces the lookup with a group literally named "1000"
// whose ID is 500.
func numericNameGroup(p *Prog) {
	named := &user.Group{Name: "1000", Gid: "500"}
	p.lookupGroup = func(name string) (*user.Group, error) {
		if name == "1000" {
			return named, nil
		}
		return testGroup(name)
	}
	p.lookupGroupID = func(id string) (*user.Group, error) {
		if id == "500" {
			return named, nil
		}
		return testGroupID(id)
	}
}

func TestRunOwnerGroup(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "sets the owner and group by names",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{"bob:devs", "file.txt"},
			want:  map[string]owner{"file.txt": {1001, 1001}},
		},
		{
			name:  "sets the owner and group by numeric IDs",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{"1001:1000", "file.txt"},
			want:  map[string]owner{"file.txt": {1001, 1000}},
		},
		{
			name:  "sets only the group",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{":devs", "file.txt"},
			want:  map[string]owner{"file.txt": {1000, 1001}},
		},
		{
			name:  "owner: sets the login group of the owner",
			files: fsx.MemFS{"file.txt": file(1001, 1001)},
			args:  []string{"alice:", "file.txt"},
			want:  map[string]owner{"file.txt": {1000, 1000}},
		},
		{
			name:  "a numeric group is accepted",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{":1001", "file.txt"},
			want:  map[string]owner{"file.txt": {1000, 1001}},
		},
		{
			name:  ":+ forces the numeric interpretation",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{":+1001", "file.txt"},
			want:  map[string]owner{"file.txt": {1000, 1001}},
		},
		{
			name:  "changes every file",
			files: fsx.MemFS{"a.txt": file(1000, 1000), "b.txt": file(1001, 1001)},
			args:  []string{":0", "a.txt", "b.txt"},
			want:  map[string]owner{"a.txt": {1000, 0}, "b.txt": {1001, 0}},
		},
		{
			name:  "a lone colon changes nothing",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{":", "file.txt"},
			want:  map[string]owner{"file.txt": {1000, 1000}},
		},
		{
			name:  "an empty operand changes nothing",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{"", "file.txt"},
			want:  map[string]owner{"file.txt": {1000, 1000}},
		},
		{
			name:     "owner: with a numeric owner is invalid",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"1001:", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: invalid spec: \"1001:\"\n",
			wantExit: 1,
		},
		{
			name:     "owner: with an unknown user is invalid",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"nosuch:", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: invalid spec: \"nosuch:\"\n",
			wantExit: 1,
		},
		{
			name:     "an unknown group with an owner is invalid",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"bob:nosuch", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: invalid group: \"bob:nosuch\"\n",
			wantExit: 1,
		},
		{
			name:     "an unknown group alone is invalid",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{":nosuch", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: invalid group: \":nosuch\"\n",
			wantExit: 1,
		},
		{
			name:     "a group spec with another colon is invalid",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"alice:devs:x", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: invalid group: \"alice:devs:x\"\n",
			wantExit: 1,
		},
		{
			name:     "an unknown user with a group is invalid",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"nosuch:devs", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: invalid user: \"nosuch:devs\"\n",
			wantExit: 1,
		},
		{
			name:  "a numeric group that names a group wins as the name",
			files: fsx.MemFS{"file.txt": file(0, 0)},
			args:  []string{":1000", "file.txt"},
			hook:  numericNameGroup,
			want:  map[string]owner{"file.txt": {0, 500}},
		},
		{
			name:  ":+ forces the numeric interpretation of the group",
			files: fsx.MemFS{"file.txt": file(0, 0)},
			args:  []string{":+1000", "file.txt"},
			hook:  numericNameGroup,
			want:  map[string]owner{"file.txt": {0, 1000}},
		},
	})
}

// tree is a small hierarchy owned by alice, rebuilt by the callers.
func tree() fsx.MemFS {
	return fsx.MemFS{
		"dir":           dir(1000, 1000),
		"dir/a.txt":     file(1000, 1000),
		"dir/sub":       dir(1000, 1000),
		"dir/sub/b.txt": file(1000, 1000),
	}
}

func TestRunRecursive(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "R changes every file below the directory",
			files: tree(),
			args:  []string{"-R", "0:0", "dir"},
			want: map[string]owner{
				"dir":           {0, 0},
				"dir/a.txt":     {0, 0},
				"dir/sub":       {0, 0},
				"dir/sub/b.txt": {0, 0},
			},
		},
		{
			name:  "R on a plain file works like without R",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{"-R", "0:0", "file.txt"},
			want:  map[string]owner{"file.txt": {0, 0}},
		},
		{
			name:  "an explicit -P behaves like the default",
			files: tree(),
			args:  []string{"-R", "-P", "0:0", "dir"},
			want: map[string]owner{
				"dir":           {0, 0},
				"dir/a.txt":     {0, 0},
				"dir/sub":       {0, 0},
				"dir/sub/b.txt": {0, 0},
			},
		},
		{
			name:     "R keeps going after a missing file",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"-R", "0:0", "missing", "file.txt"},
			want:     map[string]owner{"file.txt": {0, 0}},
			wantErr:  "chown: cannot access 'missing': file does not exist\n",
			wantExit: 1,
		},
		{
			name: "R does not change what a link of the walk points at",
			files: fsx.MemFS{
				"dir":      dir(1000, 1000),
				"dir/link": link("out", 1000, 1000),
				"out":      file(1000, 1000),
			},
			args: []string{"-R", "0:0", "dir"},
			want: map[string]owner{
				"dir":      {0, 0},
				"dir/link": {1000, 1000},
				"out":      {1000, 1000},
			},
		},
		{
			name: "R leaves a dangling link of the walk alone",
			files: fsx.MemFS{
				"dir":      dir(1000, 1000),
				"dir/link": link("missing", 1000, 1000),
			},
			args: []string{"-R", "0:0", "dir"},
			want: map[string]owner{
				"dir":      {0, 0},
				"dir/link": {1000, 1000},
			},
		},
		{
			name: "R follows a link operand to a directory, like -H",
			files: fsx.MemFS{
				"dir":       dir(1000, 1000),
				"dir/a.txt": file(1000, 1000),
				"dlink":     link("dir", 1000, 1000),
			},
			args: []string{"-R", "-H", "0:0", "dlink"},
			want: map[string]owner{
				"dir":       {0, 0},
				"dir/a.txt": {0, 0},
				"dlink":     {1000, 1000},
			},
		},
		{
			name: "R does not follow a link operand without -H or -L",
			files: fsx.MemFS{
				"dir":       dir(1000, 1000),
				"dir/a.txt": file(1000, 1000),
				"dlink":     link("dir", 1000, 1000),
			},
			args: []string{"-R", "0:0", "dlink"},
			want: map[string]owner{
				"dir":       {1000, 1000},
				"dir/a.txt": {1000, 1000},
				"dlink":     {1000, 1000},
			},
		},
		{
			name: "H follows a link operand to a plain file",
			files: fsx.MemFS{
				"file.txt": file(1000, 1000),
				"flink":    link("file.txt", 1000, 1000),
			},
			args: []string{"-R", "-H", "0:0", "flink"},
			want: map[string]owner{
				"file.txt": {0, 0},
				"flink":    {1000, 1000},
			},
		},
		{
			name: "H dereferences the links of the walk without traversing them",
			files: fsx.MemFS{
				"dir":      dir(1000, 1000),
				"dir/link": link("../out", 1000, 1000),
				"out":      file(1000, 1000),
			},
			args: []string{"-R", "-H", "0:0", "dir"},
			want: map[string]owner{
				"dir":      {0, 0},
				"dir/link": {1000, 1000},
				"out":      {0, 0},
			},
		},
		{
			name: "h leaves the links of the walk alone with H",
			files: fsx.MemFS{
				"dir":      dir(1000, 1000),
				"dir/link": link("../out", 1000, 1000),
				"out":      file(1000, 1000),
			},
			args: []string{"-R", "-h", "-H", "0:0", "dir"},
			want: map[string]owner{
				"dir":      {0, 0},
				"dir/link": {1000, 1000},
				"out":      {1000, 1000},
			},
		},
		{
			name: "L follows the links of the walk",
			files: fsx.MemFS{
				"dir":      dir(1000, 1000),
				"dir/link": link("../out", 1000, 1000),
				"out":      file(1000, 1000),
			},
			args: []string{"-R", "-L", "0:0", "dir"},
			want: map[string]owner{
				"dir":      {0, 0},
				"dir/link": {1000, 1000},
				"out":      {0, 0},
			},
		},
		{
			name: "L traverses a link to a directory of the walk",
			files: fsx.MemFS{
				"dir":       dir(1000, 1000),
				"dir/sub":   dir(1000, 1000),
				"dir/sub/b": file(1000, 1000),
				"dir/link":  link("sub", 1000, 1000),
			},
			args: []string{"-R", "-L", "0:0", "dir"},
			want: map[string]owner{
				"dir":       {0, 0},
				"dir/sub":   {0, 0},
				"dir/sub/b": {0, 0},
				"dir/link":  {1000, 1000},
			},
		},
		{
			name:     "L reports a dangling link of the walk",
			files:    fsx.MemFS{"dir": dir(1000, 1000), "dir/link": link("missing", 1000, 1000)},
			args:     []string{"-R", "-L", "0:0", "dir"},
			want:     map[string]owner{"dir": {0, 0}, "dir/link": {1000, 1000}},
			wantErr:  "chown: cannot dereference 'dir/link': file does not exist\n",
			wantExit: 1,
		},
		{
			name: "L refuses a tree with a cyclic link",
			files: fsx.MemFS{
				"d":    dir(1000, 1000),
				"d/f":  file(1000, 1000),
				"d/up": link(".", 1000, 1000),
			},
			args:     []string{"-R", "-L", "0:0", "d"},
			want:     map[string]owner{"d": {0, 0}, "d/f": {0, 0}},
			wantErr:  "chown: cycle in directory tree 'd/up'\n",
			wantExit: 1,
		},
		{
			name:  "the last of H, L and P wins",
			files: tree(),
			args:  []string{"-R", "-L", "-H", "-P", "0:0", "dir"},
			want: map[string]owner{
				"dir":           {0, 0},
				"dir/a.txt":     {0, 0},
				"dir/sub":       {0, 0},
				"dir/sub/b.txt": {0, 0},
			},
		},
	})
}

func TestRunNoDereference(t *testing.T) {
	linked := func() fsx.MemFS {
		return fsx.MemFS{
			"file.txt": file(1000, 1000),
			"link":     link("file.txt", 1000, 1000),
		}
	}

	runTests(t, []testCase{
		{
			name:  "h leaves the link itself alone",
			files: linked(),
			args:  []string{"-h", "0:0", "link"},
			want: map[string]owner{
				"file.txt": {1000, 1000},
				"link":     {1000, 1000},
			},
		},
		{
			name:  "the default changes the referent",
			files: linked(),
			args:  []string{"0:0", "link"},
			want: map[string]owner{
				"file.txt": {0, 0},
				"link":     {1000, 1000},
			},
		},
		{
			name:  "--dereference changes the referent",
			files: linked(),
			args:  []string{"--dereference", "0:0", "link"},
			want: map[string]owner{
				"file.txt": {0, 0},
				"link":     {1000, 1000},
			},
		},
		{
			name:  "the last of -h and --dereference wins",
			files: linked(),
			args:  []string{"--dereference", "-h", "0:0", "link"},
			want: map[string]owner{
				"file.txt": {1000, 1000},
				"link":     {1000, 1000},
			},
		},
		{
			name:  "the long form of -h works",
			files: linked(),
			args:  []string{"--no-dereference", "0:0", "link"},
			want: map[string]owner{
				"file.txt": {1000, 1000},
				"link":     {1000, 1000},
			},
		},
		{
			name:  "h still changes a regular file",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{"-h", "0:0", "file.txt"},
			want:  map[string]owner{"file.txt": {0, 0}},
		},
		{
			name:  "a chain of links is followed to the file",
			files: fsx.MemFS{"file.txt": file(1000, 1000), "l2": link("file.txt", 1000, 1000), "l1": link("l2", 1000, 1000)},
			args:  []string{"0:0", "l1"},
			want: map[string]owner{
				"file.txt": {0, 0},
				"l1":       {1000, 1000},
				"l2":       {1000, 1000},
			},
		},
		{
			name:     "a dangling link is an error by default",
			files:    fsx.MemFS{"dangling": link("missing", 1000, 1000)},
			args:     []string{"0:0", "dangling"},
			want:     map[string]owner{"dangling": {1000, 1000}},
			wantErr:  "chown: cannot dereference 'dangling': file does not exist\n",
			wantExit: 1,
		},
		{
			name:  "a dangling link is left alone with -h",
			files: fsx.MemFS{"dangling": link("missing", 1000, 1000)},
			args:  []string{"-h", "0:0", "dangling"},
			want:  map[string]owner{"dangling": {1000, 1000}},
		},
	})
}

func TestRunVerbose(t *testing.T) {
	runTests(t, []testCase{
		{
			name:    "v reports a change of owner",
			files:   fsx.MemFS{"file.txt": file(1000, 1000)},
			args:    []string{"-v", "bob", "file.txt"},
			want:    map[string]owner{"file.txt": {1001, 1000}},
			wantOut: "changed ownership of 'file.txt' from alice to bob\n",
		},
		{
			name:    "v reports a change of owner and group",
			files:   fsx.MemFS{"file.txt": file(1000, 1000)},
			args:    []string{"-v", "bob:devs", "file.txt"},
			want:    map[string]owner{"file.txt": {1001, 1001}},
			wantOut: "changed ownership of 'file.txt' from alice:staff to bob:devs\n",
		},
		{
			name:    "v reports a change of group",
			files:   fsx.MemFS{"file.txt": file(1000, 1000)},
			args:    []string{"-v", ":devs", "file.txt"},
			want:    map[string]owner{"file.txt": {1000, 1001}},
			wantOut: "changed group of 'file.txt' from staff to devs\n",
		},
		{
			name:    "v reports a retained owner",
			files:   fsx.MemFS{"file.txt": file(1000, 1000)},
			args:    []string{"-v", "alice", "file.txt"},
			want:    map[string]owner{"file.txt": {1000, 1000}},
			wantOut: "ownership of 'file.txt' retained as alice\n",
		},
		{
			name:    "v reports a retained group",
			files:   fsx.MemFS{"file.txt": file(1000, 1000)},
			args:    []string{"-v", ":staff", "file.txt"},
			want:    map[string]owner{"file.txt": {1000, 1000}},
			wantOut: "group of 'file.txt' retained as staff\n",
		},
		{
			name:    "v reports a lone colon as a retained ownership",
			files:   fsx.MemFS{"file.txt": file(1000, 1000)},
			args:    []string{"-v", ":", "file.txt"},
			want:    map[string]owner{"file.txt": {1000, 1000}},
			wantOut: "ownership of 'file.txt' retained\n",
		},
		{
			name:    "v reports unknown old IDs as numbers",
			files:   fsx.MemFS{"file.txt": file(500, 500)},
			args:    []string{"-v", "bob", "file.txt"},
			want:    map[string]owner{"file.txt": {1001, 500}},
			wantOut: "changed ownership of 'file.txt' from 500 to bob\n",
		},
		{
			name:    "v reports a + prefixed owner as its number",
			files:   fsx.MemFS{"file.txt": file(1000, 1000)},
			args:    []string{"-v", "+1001", "file.txt"},
			want:    map[string]owner{"file.txt": {1001, 1000}},
			wantOut: "changed ownership of 'file.txt' from alice to 1001\n",
		},
		{
			name:    "c reports only the files whose ownership changes",
			files:   fsx.MemFS{"a.txt": file(1000, 1000), "b.txt": file(0, 1000)},
			args:    []string{"-c", "0", "a.txt", "b.txt"},
			want:    map[string]owner{"a.txt": {0, 1000}, "b.txt": {0, 1000}},
			wantOut: "changed ownership of 'a.txt' from alice to 0\n",
		},
		{
			name:  "c reports nothing when nothing changes",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{"-c", "alice", "file.txt"},
			want:  map[string]owner{"file.txt": {1000, 1000}},
		},
		{
			name:  "without c or v nothing is reported",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{"bob", "file.txt"},
			want:  map[string]owner{"file.txt": {1001, 1000}},
		},
		{
			name:    "the last of c and v wins",
			files:   fsx.MemFS{"file.txt": file(1000, 1000)},
			args:    []string{"-c", "-v", "alice", "file.txt"},
			want:    map[string]owner{"file.txt": {1000, 1000}},
			wantOut: "ownership of 'file.txt' retained as alice\n",
		},
		{
			name:  "the last of v and c wins",
			files: fsx.MemFS{"file.txt": file(1000, 1000)},
			args:  []string{"-v", "-c", "alice", "file.txt"},
			want:  map[string]owner{"file.txt": {1000, 1000}},
		},
		{
			name:    "the long options work",
			files:   fsx.MemFS{"file.txt": file(1000, 1000)},
			args:    []string{"--verbose", "0", "file.txt"},
			want:    map[string]owner{"file.txt": {0, 1000}},
			wantOut: "changed ownership of 'file.txt' from alice to 0\n",
		},
		{
			name:    "v reports the operand as given",
			files:   fsx.MemFS{"file.txt": file(1000, 1000)},
			args:    []string{"-v", "bob", "./file.txt"},
			want:    map[string]owner{"file.txt": {1001, 1000}},
			wantOut: "changed ownership of './file.txt' from alice to bob\n",
		},
	})
}

func TestRunVerboseRecursive(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "v reports every file of the walk",
			files: tree(),
			args:  []string{"-Rv", "0:0", "dir"},
			want: map[string]owner{
				"dir":           {0, 0},
				"dir/a.txt":     {0, 0},
				"dir/sub":       {0, 0},
				"dir/sub/b.txt": {0, 0},
			},
			wantOut: "changed ownership of 'dir' from alice:staff to 0:0\n" +
				"changed ownership of 'dir/a.txt' from alice:staff to 0:0\n" +
				"changed ownership of 'dir/sub' from alice:staff to 0:0\n" +
				"changed ownership of 'dir/sub/b.txt' from alice:staff to 0:0\n",
		},
		{
			name:  "c reports only the files whose ownership changes",
			files: tree(),
			args:  []string{"-Rc", "alice", "dir"},
			want: map[string]owner{
				"dir":           {1000, 1000},
				"dir/a.txt":     {1000, 1000},
				"dir/sub":       {1000, 1000},
				"dir/sub/b.txt": {1000, 1000},
			},
		},
		{
			name:  "v doesn't report the links the walk leaves alone",
			files: fsx.MemFS{"dir": dir(1000, 1000), "dir/link": link("out", 1000, 1000), "out": file(1000, 1000)},
			args:  []string{"-Rv", "0:0", "dir"},
			want: map[string]owner{
				"dir":      {0, 0},
				"dir/link": {1000, 1000},
				"out":      {1000, 1000},
			},
			wantOut: "changed ownership of 'dir' from alice:staff to 0:0\n",
		},
	})
}

func TestRunFrom(t *testing.T) {
	two := func() fsx.MemFS {
		return fsx.MemFS{"a.txt": file(1000, 1000), "b.txt": file(1001, 1001)}
	}

	runTests(t, []testCase{
		{
			name:  "from matches the owner",
			files: two(),
			args:  []string{"--from=alice", "0:0", "a.txt", "b.txt"},
			want:  map[string]owner{"a.txt": {0, 0}, "b.txt": {1001, 1001}},
		},
		{
			name:  "from matches the owner and the group",
			files: two(),
			args:  []string{"--from=alice:staff", "0:0", "a.txt", "b.txt"},
			want:  map[string]owner{"a.txt": {0, 0}, "b.txt": {1001, 1001}},
		},
		{
			name:  "from=OWNER: matches the owner across every group",
			files: fsx.MemFS{"a.txt": file(1000, 1000), "b.txt": file(1000, 1001), "c.txt": file(1001, 1000)},
			args:  []string{"--from=alice:", "0:0", "a.txt", "b.txt", "c.txt"},
			want:  map[string]owner{"a.txt": {0, 0}, "b.txt": {0, 0}, "c.txt": {1001, 1000}},
		},
		{
			name:  "from=UID: matches a numeric owner across every group",
			files: fsx.MemFS{"a.txt": file(1000, 1000), "b.txt": file(1000, 1001)},
			args:  []string{"--from=1000:", "0:0", "a.txt", "b.txt"},
			want:  map[string]owner{"a.txt": {0, 0}, "b.txt": {0, 0}},
		},
		{
			name:  "from=:GROUP matches the group across every owner",
			files: fsx.MemFS{"a.txt": file(1000, 1001), "b.txt": file(1001, 1001), "c.txt": file(1000, 1000)},
			args:  []string{"--from=:devs", "0:0", "a.txt", "b.txt", "c.txt"},
			want:  map[string]owner{"a.txt": {0, 0}, "b.txt": {0, 0}, "c.txt": {1000, 1000}},
		},
		{
			name:  "from matches only the group",
			files: two(),
			args:  []string{"--from=:staff", "0:0", "a.txt", "b.txt"},
			want:  map[string]owner{"a.txt": {0, 0}, "b.txt": {1001, 1001}},
		},
		{
			name:  "from does not match a different group",
			files: two(),
			args:  []string{"--from=alice:devs", "0:0", "a.txt", "b.txt"},
			want:  map[string]owner{"a.txt": {1000, 1000}, "b.txt": {1001, 1001}},
		},
		{
			name:  "from does not match a different owner",
			files: two(),
			args:  []string{"--from=bob", "0:0", "a.txt", "b.txt"},
			want:  map[string]owner{"a.txt": {1000, 1000}, "b.txt": {0, 0}},
		},
		{
			name:  "from matches a numeric ID",
			files: two(),
			args:  []string{"--from=+1001", "0:0", "a.txt", "b.txt"},
			want:  map[string]owner{"a.txt": {1000, 1000}, "b.txt": {0, 0}},
		},
		{
			name:  "from works with R",
			files: fsx.MemFS{"d": dir(1000, 1000), "d/f": file(1001, 1001)},
			args:  []string{"-R", "--from=alice", "0:0", "d"},
			want:  map[string]owner{"d": {0, 0}, "d/f": {1001, 1001}},
		},
		{
			name:  "v prints nothing for the files from skips",
			files: fsx.MemFS{"a.txt": file(1000, 1000)},
			args:  []string{"-v", "--from=bob", "0:0", "a.txt"},
			want:  map[string]owner{"a.txt": {1000, 1000}},
		},
		{
			name:     "an invalid from is an error",
			files:    fsx.MemFS{"a.txt": file(1000, 1000)},
			args:     []string{"--from=nosuch", "0:0", "a.txt"},
			want:     map[string]owner{"a.txt": {1000, 1000}},
			wantErr:  "chown: invalid user: \"nosuch\"\n",
			wantExit: 1,
		},
	})
}

func TestRunReference(t *testing.T) {
	ref := func() fsx.MemFS {
		return fsx.MemFS{"ref": file(1001, 1001), "file.txt": file(1000, 1000)}
	}

	runTests(t, []testCase{
		{
			name:  "uses the ownership of the reference file",
			files: ref(),
			args:  []string{"--reference=ref", "file.txt"},
			want:  map[string]owner{"ref": {1001, 1001}, "file.txt": {1001, 1001}},
		},
		{
			name:  "the reference can be a separate argument",
			files: ref(),
			args:  []string{"--reference", "ref", "file.txt"},
			want:  map[string]owner{"ref": {1001, 1001}, "file.txt": {1001, 1001}},
		},
		{
			name:  "the ownership applies to every file",
			files: fsx.MemFS{"ref": file(1001, 1001), "a.txt": file(0, 0), "b.txt": file(0, 0)},
			args:  []string{"--reference=ref", "a.txt", "b.txt"},
			want: map[string]owner{
				"ref": {1001, 1001}, "a.txt": {1001, 1001}, "b.txt": {1001, 1001},
			},
		},
		{
			name:  "a reference to a symbolic link is dereferenced",
			files: fsx.MemFS{"ref": file(1001, 1001), "rlink": link("ref", 0, 0), "file.txt": file(1000, 1000)},
			args:  []string{"--reference=rlink", "file.txt"},
			want: map[string]owner{
				"ref": {1001, 1001}, "rlink": {0, 0}, "file.txt": {1001, 1001},
			},
		},
		{
			name:    "v reports the names of the reference",
			files:   ref(),
			args:    []string{"-v", "--reference=ref", "file.txt"},
			want:    map[string]owner{"ref": {1001, 1001}, "file.txt": {1001, 1001}},
			wantOut: "changed ownership of 'file.txt' from alice:staff to bob:devs\n",
		},
		{
			name:     "a missing reference file is an error",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"--reference=missing", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: failed to get attributes of missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "no operands is a usage error",
			files:    fsx.MemFS{"ref": file(1001, 1001)},
			args:     []string{"--reference=ref"},
			want:     map[string]owner{"ref": {1001, 1001}},
			wantErr:  "chown: missing operand\nTry 'chown --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:  "every operand is a file with a reference",
			files: fsx.MemFS{"ref": file(1001, 1001), "file.txt": file(1000, 1000)},
			args:  []string{"--reference=ref", "1000", "file.txt"},
			want: map[string]owner{
				"ref": {1001, 1001}, "file.txt": {1001, 1001},
			},
			wantErr:  "chown: cannot access '1000': file does not exist\n",
			wantExit: 1,
		},
		{
			name:  "a reference works with R",
			files: fsx.MemFS{"ref": file(1001, 1001), "d": dir(1000, 1000), "d/f": file(1000, 1000)},
			args:  []string{"-R", "--reference=ref", "d"},
			want: map[string]owner{
				"ref": {1001, 1001}, "d": {1001, 1001}, "d/f": {1001, 1001},
			},
		},
	})
}

func TestRunGeneral(t *testing.T) {
	runTests(t, []testCase{
		{
			name:     "a missing operand is an error",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     nil,
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: missing operand\nTry 'chown --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a missing file operand is an error",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"alice"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: missing file operand\nTry 'chown --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid option is an error",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"-x", "alice", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: invalid option -- 'x'\nTry 'chown --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid option in a group is an error",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"-Rx", "alice", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: invalid option -- 'x'\nTry 'chown --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"--bogus", "alice", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: unrecognized option '--bogus'\nTry 'chown --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a missing option argument is an error",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"--from"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: option '--from' requires an argument\nTry 'chown --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an owner starting with a dash can follow a double dash",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"--", "-42", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: invalid user: \"-42\"\n",
			wantExit: 1,
		},
		{
			name:  "a file named like an option can follow a double dash",
			files: fsx.MemFS{"-w": file(1000, 1000)},
			args:  []string{"--", "0", "-w"},
			want:  map[string]owner{"-w": {0, 1000}},
		},
		{
			name:  "options can be grouped",
			files: tree(),
			args:  []string{"-Rv", "0:0", "dir"},
			want: map[string]owner{
				"dir":           {0, 0},
				"dir/a.txt":     {0, 0},
				"dir/sub":       {0, 0},
				"dir/sub/b.txt": {0, 0},
			},
			wantOut: "changed ownership of 'dir' from alice:staff to 0:0\n" +
				"changed ownership of 'dir/a.txt' from alice:staff to 0:0\n" +
				"changed ownership of 'dir/sub' from alice:staff to 0:0\n" +
				"changed ownership of 'dir/sub/b.txt' from alice:staff to 0:0\n",
		},
		{
			name:     "a missing file is an error, but the others are changed",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"0", "missing", "file.txt"},
			want:     map[string]owner{"file.txt": {0, 1000}},
			wantErr:  "chown: cannot access 'missing': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "f suppresses the diagnostics but keeps the status",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"-f", "0", "missing", "file.txt"},
			want:     map[string]owner{"file.txt": {0, 1000}},
			wantExit: 1,
		},
	})
}

func TestRunPreserveRoot(t *testing.T) {
	failsafe := "chown: it is dangerous to operate recursively on '/'\n" +
		"chown: use --no-preserve-root to override this failsafe\n"

	runTests(t, []testCase{
		{
			name:     "R on the root is not refused by default",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"-R", "0:0", "/"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: cannot access '/': invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "preserve-root refuses a recursive run on /",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"--preserve-root", "-R", "0:0", "/"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  failsafe,
			wantExit: 1,
		},
		{
			name:     "preserve-root also catches a cleaned / operand",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"-R", "--preserve-root", "0:0", "/usr/.."},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  failsafe,
			wantExit: 1,
		},
		{
			name:     "no-preserve-root overrides the failsafe",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"--preserve-root", "--no-preserve-root", "-R", "0:0", "/"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: cannot access '/': invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "the last of the two options wins",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"--no-preserve-root", "--preserve-root", "-R", "0:0", "/"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  failsafe,
			wantExit: 1,
		},
		{
			name:     "without R the option has no effect",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"--preserve-root", "0:0", "/"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: cannot access '/': invalid argument\n",
			wantExit: 1,
		},
		{
			name:  "preserve-root leaves the other hierarchies alone",
			files: fsx.MemFS{"d": dir(1000, 1000), "d/f": file(1000, 1000)},
			args:  []string{"--preserve-root", "-R", "0:0", "d"},
			want:  map[string]owner{"d": {0, 0}, "d/f": {0, 0}},
		},
		{
			name:  "preserve-root leaves a followed link to a hierarchy alone",
			files: fsx.MemFS{"dir": dir(1000, 1000), "dlink": link("dir", 1000, 1000)},
			args:  []string{"--preserve-root", "-R", "-H", "0:0", "dlink"},
			want:  map[string]owner{"dir": {0, 0}, "dlink": {1000, 1000}},
		},
		{
			name:     "preserve-root refuses a link operand pointing at / with H",
			files:    fsx.MemFS{"rlink": link("/", 1000, 1000), "file.txt": file(1000, 1000)},
			args:     []string{"--preserve-root", "-R", "-H", "0:0", "rlink"},
			want:     map[string]owner{"rlink": {1000, 1000}, "file.txt": {1000, 1000}},
			wantErr:  failsafe,
			wantExit: 1,
		},
		{
			name:     "preserve-root refuses a link operand pointing at / with L",
			files:    fsx.MemFS{"rlink": link("/", 1000, 1000), "file.txt": file(1000, 1000)},
			args:     []string{"--preserve-root", "-R", "-L", "0:0", "rlink"},
			want:     map[string]owner{"rlink": {1000, 1000}, "file.txt": {1000, 1000}},
			wantErr:  failsafe,
			wantExit: 1,
		},
		{
			name:     "preserve-root refuses a chain of links pointing at /",
			files:    fsx.MemFS{"l2": link("/", 1000, 1000), "l1": link("l2", 1000, 1000)},
			args:     []string{"--preserve-root", "-R", "-L", "0:0", "l1"},
			want:     map[string]owner{"l1": {1000, 1000}, "l2": {1000, 1000}},
			wantErr:  failsafe,
			wantExit: 1,
		},
		{
			name:  "a link operand pointing at / is not refused by default",
			files: fsx.MemFS{"rlink": link("/", 1000, 1000), "file.txt": file(1000, 1000)},
			args:  []string{"-R", "-H", "0:0", "rlink"},
			want:  map[string]owner{"file.txt": {0, 0}, "rlink": {1000, 1000}},
		},
	})
}

func TestRunLongOptions(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "recursive changes every file below the directory",
			files: fsx.MemFS{"d": dir(1000, 1000), "d/f": file(1000, 1000)},
			args:  []string{"--recursive", "0:0", "d"},
			want:  map[string]owner{"d": {0, 0}, "d/f": {0, 0}},
		},
		{
			name:     "silent suppresses the diagnostics",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"--silent", "0", "missing", "file.txt"},
			want:     map[string]owner{"file.txt": {0, 1000}},
			wantExit: 1,
		},
		{
			name:     "quiet suppresses the diagnostics too",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"--quiet", "0", "missing", "file.txt"},
			want:     map[string]owner{"file.txt": {0, 1000}},
			wantExit: 1,
		},
	})
}

func TestRunConflict(t *testing.T) {
	runTests(t, []testCase{
		{
			name:     "R with dereference needs a traversal mode",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"-R", "--dereference", "0:0", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: -R --dereference requires either -H or -L\n",
			wantExit: 1,
		},
		{
			name:     "R with -P and dereference is refused too",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"-R", "-P", "--dereference", "0:0", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: -R --dereference requires either -H or -L\n",
			wantExit: 1,
		},
		{
			name:     "the option order doesn't matter for the check",
			files:    fsx.MemFS{"file.txt": file(1000, 1000)},
			args:     []string{"--dereference", "-R", "0:0", "file.txt"},
			want:     map[string]owner{"file.txt": {1000, 1000}},
			wantErr:  "chown: -R --dereference requires either -H or -L\n",
			wantExit: 1,
		},
		{
			name:  "R with dereference and H dereferences the walk",
			files: fsx.MemFS{"d": dir(1000, 1000), "d/link": link("../out", 1000, 1000), "out": file(1000, 1000)},
			args:  []string{"-R", "--dereference", "-H", "0:0", "d"},
			want: map[string]owner{
				"d":      {0, 0},
				"d/link": {1000, 1000},
				"out":    {0, 0},
			},
		},
	})
}

// errDenied is the failure a failFS reports for every chown.
var errDenied = errors.New("permission denied")

// failFS is a file system that refuses to change the ownership.
type failFS struct {
	fsx.FS
}

func (failFS) Chown(string, int, int) error { return errDenied }

func (f failFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(f.FS, name)
}

func TestRunFailingFS(t *testing.T) {
	newProg := func(files fsx.FS, stdout, stderr io.Writer) *Prog {
		p := New(&coreutils.Runtime{Stdout: stdout, Stderr: stderr, FS: files}).(*Prog)
		p.lookupUser, p.lookupGroup = testUser, testGroup
		p.lookupUserID, p.lookupGroupID = testUserID, testGroupID
		return p
	}

	t.Run("a failed change is reported and keeps the status", func(t *testing.T) {
		stderr := new(bytes.Buffer)
		p := newProg(failFS{fsx.MemFS{"f": file(1000, 1000)}}, io.Discard, stderr)
		err := p.Run(t.Context(), "0", "f")
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
		if got, want := stderr.String(), "chown: changing ownership of 'f': permission denied\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("a group change is reported as one", func(t *testing.T) {
		stderr := new(bytes.Buffer)
		p := newProg(failFS{fsx.MemFS{"f": file(1000, 1000)}}, io.Discard, stderr)
		err := p.Run(t.Context(), ":devs", "f")
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
		if got, want := stderr.String(), "chown: changing group of 'f': permission denied\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("f silences the failure but keeps the status", func(t *testing.T) {
		stderr := new(bytes.Buffer)
		p := newProg(failFS{fsx.MemFS{"f": file(1000, 1000)}}, io.Discard, stderr)
		err := p.Run(t.Context(), "-f", "0", "f")
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
	})

	t.Run("R keeps walking below a directory that fails", func(t *testing.T) {
		stderr := new(bytes.Buffer)
		p := newProg(failFS{fsx.MemFS{"d": dir(1000, 1000), "d/f.txt": file(1000, 1000)}}, io.Discard, stderr)
		err := p.Run(t.Context(), "-R", "0", "d")
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
		want := "chown: changing ownership of 'd': permission denied\n" +
			"chown: changing ownership of 'd/f.txt': permission denied\n"
		if got := stderr.String(); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})
}

// noChownFS is a file system without the ability to change the ownership.
type noChownFS struct {
	fsx.FS
}

func TestRunFSWithoutChown(t *testing.T) {
	files := noChownFS{fsx.MemFS{"file.txt": file(1000, 1000)}}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: stderr, FS: files}
	err := New(rt).Run(t.Context(), "0", "file.txt")
	if code := exitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if got, want := stderr.String(), "chown: changing ownership is not supported\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
	fi, err := files.Lstat("file.txt")
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if uid, gid, _ := fsx.Owner(fi); uid != 1000 || gid != 1000 {
		t.Errorf("ownership = %d:%d, want 1000:1000", uid, gid)
	}
}

func TestRunOsFS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("there is no numeric ownership on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root can change any ownership")
	}

	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "ok.txt"), nil, 0o644))
	must(os.WriteFile(filepath.Join(dir, "denied.txt"), nil, 0o644))

	uid := os.Getuid()
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: stderr, FS: fsx.OsFS(dir)}
	if err := New(rt).Run(t.Context(), strconv.Itoa(uid), "ok.txt"); err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	if err := New(rt).Run(t.Context(), "0", "denied.txt"); err == nil {
		t.Error("Run() error = nil, want a failure")
	} else if code := exitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1", code)
	}
}

func TestRunOsFSSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("there is no numeric ownership on Windows")
	}

	// The operating system Chown follows symbolic links, so a link the run
	// would act on itself is left alone: neither it nor its referent is
	// changed, as the apply comment documents.
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, nil, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "link.txt")); err != nil {
		t.Fatalf("setup: %v", err)
	}

	before := func(name string) (int, int) {
		fi, err := os.Lstat(name)
		if err != nil {
			t.Fatalf("Lstat(%q) error = %v", name, err)
		}
		uid, gid, ok := fsx.Owner(fi)
		if !ok {
			t.Fatalf("Owner(%q) = unknown", name)
		}
		return uid, gid
	}
	link := filepath.Join(dir, "link.txt")
	targetUid, targetGid := before(target)
	linkUid, linkGid := before(link)

	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: new(bytes.Buffer), FS: fsx.OsFS(dir)}
	if err := New(rt).Run(t.Context(), "-h", "0:0", "link.txt"); err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	if uid, gid := before(target); uid != targetUid || gid != targetGid {
		t.Errorf("referent ownership = %d:%d, want %d:%d", uid, gid, targetUid, targetGid)
	}
	if uid, gid := before(link); uid != linkUid || gid != linkGid {
		t.Errorf("link ownership = %d:%d, want it left alone as %d:%d", uid, gid, linkUid, linkGid)
	}
}

func TestRunOsFSPreserveRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("there is no numeric ownership on Windows")
	}

	// A command line link standing for the root is refused before anything
	// is changed, so the run works the same without privileges.
	dir := t.TempDir()
	if err := os.Symlink("/", filepath.Join(dir, "rootlink")); err != nil {
		t.Fatalf("setup: %v", err)
	}

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: stderr, FS: fsx.OsFS(dir)}
	err := New(rt).Run(t.Context(), "--preserve-root", "-R", "-H", "0:0", "rootlink")
	if code := exitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	want := "chown: it is dangerous to operate recursively on '/'\n" +
		"chown: use --no-preserve-root to override this failsafe\n"
	if got := stderr.String(); got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}

func TestRunWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only behavior")
	}

	t.Run("a numeric chown is a no-op", func(t *testing.T) {
		dir := t.TempDir()
		name := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(name, nil, 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: stderr, FS: fsx.OsFS(dir)}
		if err := New(rt).Run(t.Context(), "0:0", "file.txt"); err != nil {
			t.Errorf("Run() error = %v, stderr = %q", err, stderr)
		}
	})

	t.Run("backslash operands are normalized", func(t *testing.T) {
		files := fsx.MemFS{"sub": dir(0, 0), "sub/file.txt": file(0, 0)}
		runCase(t, testCase{
			files: files,
			args:  []string{"1000:1000", "sub\\file.txt"},
			want:  map[string]owner{"sub/file.txt": {1000, 1000}},
		})
	})
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
		want := "chown (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
