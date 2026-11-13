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

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
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

// testCase is the fields of a testutil.Case plus the Prog hooks the tests
// need, the file fixtures, and the ownerships the files must have after the
// run. runCase stays local: the user lookups have to be stubbed on the Prog
// before Run, which testutil.Run can't do.
type testCase struct {
	Name     string
	Args     []string
	Want     string
	WantErr  string
	WantExit int

	Hook       func(*Prog)
	Files      fsx.MemFS
	WantOwners map[string]owner
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: stdout,
		Stderr: stderr,
		FS:     test.Files,
	}
	p := New(rt).(*Prog)
	p.lookupUser, p.lookupGroup = testUser, testGroup
	p.lookupUserID, p.lookupGroupID = testUserID, testGroupID
	if test.Hook != nil {
		test.Hook(p)
	}

	err := p.Run(t.Context(), test.Args...)

	if code := testutil.ExitCode(t, err); code != test.WantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.WantExit, err)
	}
	if got := stdout.String(); got != test.Want {
		t.Errorf("Run() stdout = %q, want %q", got, test.Want)
	}
	if got := stderr.String(); got != test.WantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.WantErr)
	}

	for name, want := range test.WantOwners {
		fi, err := test.Files.Lstat(name)
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

// runTests runs the given cases as subtests.
func runTests(t *testing.T, tests []testCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOwner(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:       "sets the owner by name",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"bob", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1001, 1000}},
		},
		{
			Name:       "sets the owner by numeric ID",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"1001", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1001, 1000}},
		},
		{
			Name:       "a + prefixed owner is forced numeric",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"+1001", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1001, 1000}},
		},
		{
			Name:       "sets the owner of every file",
			Files:      fsx.MemFS{"a.txt": file(1000, 1000), "b.txt": file(1000, 1000)},
			Args:       []string{"0", "a.txt", "b.txt"},
			WantOwners: map[string]owner{"a.txt": {0, 1000}, "b.txt": {0, 1000}},
		},
		{
			Name:       "keeps the group",
			Files:      fsx.MemFS{"file.txt": file(1000, 1001)},
			Args:       []string{"0", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {0, 1001}},
		},
		{
			Name:       "changes a directory",
			Files:      fsx.MemFS{"d": dir(1000, 1000)},
			Args:       []string{"0", "d"},
			WantOwners: map[string]owner{"d": {0, 1000}},
		},
		{
			Name:       "an unknown user is an error",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"nosuch", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: invalid user: \"nosuch\"\n",
			WantExit:   1,
		},
		{
			Name:       "a + prefixed non numeric owner is an error",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"+x", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: invalid user: \"+x\"\n",
			WantExit:   1,
		},
		{
			Name:       "a numeric owner that names a user wins as the name",
			Files:      fsx.MemFS{"file.txt": file(0, 0)},
			Args:       []string{"1000", "file.txt"},
			Hook:       numericNameUser,
			WantOwners: map[string]owner{"file.txt": {500, 0}},
		},
		{
			Name:       "+ forces the numeric interpretation",
			Files:      fsx.MemFS{"file.txt": file(0, 0)},
			Args:       []string{"+1000", "file.txt"},
			Hook:       numericNameUser,
			WantOwners: map[string]owner{"file.txt": {1000, 0}},
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
			Name:       "sets the owner and group by names",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"bob:devs", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1001, 1001}},
		},
		{
			Name:       "sets the owner and group by numeric IDs",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"1001:1000", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1001, 1000}},
		},
		{
			Name:       "sets only the group",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{":devs", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1001}},
		},
		{
			Name:       "owner: sets the login group of the owner",
			Files:      fsx.MemFS{"file.txt": file(1001, 1001)},
			Args:       []string{"alice:", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
		},
		{
			Name:       "a numeric group is accepted",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{":1001", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1001}},
		},
		{
			Name:       ":+ forces the numeric interpretation",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{":+1001", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1001}},
		},
		{
			Name:       "changes every file",
			Files:      fsx.MemFS{"a.txt": file(1000, 1000), "b.txt": file(1001, 1001)},
			Args:       []string{":0", "a.txt", "b.txt"},
			WantOwners: map[string]owner{"a.txt": {1000, 0}, "b.txt": {1001, 0}},
		},
		{
			Name:       "a lone colon changes nothing",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{":", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
		},
		{
			Name:       "an empty operand changes nothing",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
		},
		{
			Name:       "owner: with a numeric owner is invalid",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"1001:", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: invalid spec: \"1001:\"\n",
			WantExit:   1,
		},
		{
			Name:       "owner: with an unknown user is invalid",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"nosuch:", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: invalid spec: \"nosuch:\"\n",
			WantExit:   1,
		},
		{
			Name:       "an unknown group with an owner is invalid",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"bob:nosuch", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: invalid group: \"bob:nosuch\"\n",
			WantExit:   1,
		},
		{
			Name:       "an unknown group alone is invalid",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{":nosuch", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: invalid group: \":nosuch\"\n",
			WantExit:   1,
		},
		{
			Name:       "a group spec with another colon is invalid",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"alice:devs:x", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: invalid group: \"alice:devs:x\"\n",
			WantExit:   1,
		},
		{
			Name:       "an unknown user with a group is invalid",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"nosuch:devs", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: invalid user: \"nosuch:devs\"\n",
			WantExit:   1,
		},
		{
			Name:       "a numeric group that names a group wins as the name",
			Files:      fsx.MemFS{"file.txt": file(0, 0)},
			Args:       []string{":1000", "file.txt"},
			Hook:       numericNameGroup,
			WantOwners: map[string]owner{"file.txt": {0, 500}},
		},
		{
			Name:       ":+ forces the numeric interpretation of the group",
			Files:      fsx.MemFS{"file.txt": file(0, 0)},
			Args:       []string{":+1000", "file.txt"},
			Hook:       numericNameGroup,
			WantOwners: map[string]owner{"file.txt": {0, 1000}},
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
			Name:  "R changes every file below the directory",
			Files: tree(),
			Args:  []string{"-R", "0:0", "dir"},
			WantOwners: map[string]owner{
				"dir":           {0, 0},
				"dir/a.txt":     {0, 0},
				"dir/sub":       {0, 0},
				"dir/sub/b.txt": {0, 0},
			},
		},
		{
			Name:       "R on a plain file works like without R",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-R", "0:0", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {0, 0}},
		},
		{
			Name:  "an explicit -P behaves like the default",
			Files: tree(),
			Args:  []string{"-R", "-P", "0:0", "dir"},
			WantOwners: map[string]owner{
				"dir":           {0, 0},
				"dir/a.txt":     {0, 0},
				"dir/sub":       {0, 0},
				"dir/sub/b.txt": {0, 0},
			},
		},
		{
			Name:       "R keeps going after a missing file",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-R", "0:0", "missing", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {0, 0}},
			WantErr:    "chown: cannot access 'missing': file does not exist\n",
			WantExit:   1,
		},
		{
			Name: "R does not change what a link of the walk points at",
			Files: fsx.MemFS{
				"dir":      dir(1000, 1000),
				"dir/link": link("out", 1000, 1000),
				"out":      file(1000, 1000),
			},
			Args: []string{"-R", "0:0", "dir"},
			WantOwners: map[string]owner{
				"dir":      {0, 0},
				"dir/link": {1000, 1000},
				"out":      {1000, 1000},
			},
		},
		{
			Name: "R leaves a dangling link of the walk alone",
			Files: fsx.MemFS{
				"dir":      dir(1000, 1000),
				"dir/link": link("missing", 1000, 1000),
			},
			Args: []string{"-R", "0:0", "dir"},
			WantOwners: map[string]owner{
				"dir":      {0, 0},
				"dir/link": {1000, 1000},
			},
		},
		{
			Name: "R follows a link operand to a directory, like -H",
			Files: fsx.MemFS{
				"dir":       dir(1000, 1000),
				"dir/a.txt": file(1000, 1000),
				"dlink":     link("dir", 1000, 1000),
			},
			Args: []string{"-R", "-H", "0:0", "dlink"},
			WantOwners: map[string]owner{
				"dir":       {0, 0},
				"dir/a.txt": {0, 0},
				"dlink":     {1000, 1000},
			},
		},
		{
			Name: "R does not follow a link operand without -H or -L",
			Files: fsx.MemFS{
				"dir":       dir(1000, 1000),
				"dir/a.txt": file(1000, 1000),
				"dlink":     link("dir", 1000, 1000),
			},
			Args: []string{"-R", "0:0", "dlink"},
			WantOwners: map[string]owner{
				"dir":       {1000, 1000},
				"dir/a.txt": {1000, 1000},
				"dlink":     {1000, 1000},
			},
		},
		{
			Name: "H follows a link operand to a plain file",
			Files: fsx.MemFS{
				"file.txt": file(1000, 1000),
				"flink":    link("file.txt", 1000, 1000),
			},
			Args: []string{"-R", "-H", "0:0", "flink"},
			WantOwners: map[string]owner{
				"file.txt": {0, 0},
				"flink":    {1000, 1000},
			},
		},
		{
			Name: "H dereferences the links of the walk without traversing them",
			Files: fsx.MemFS{
				"dir":      dir(1000, 1000),
				"dir/link": link("../out", 1000, 1000),
				"out":      file(1000, 1000),
			},
			Args: []string{"-R", "-H", "0:0", "dir"},
			WantOwners: map[string]owner{
				"dir":      {0, 0},
				"dir/link": {1000, 1000},
				"out":      {0, 0},
			},
		},
		{
			Name: "h leaves the links of the walk alone with H",
			Files: fsx.MemFS{
				"dir":      dir(1000, 1000),
				"dir/link": link("../out", 1000, 1000),
				"out":      file(1000, 1000),
			},
			Args: []string{"-R", "-h", "-H", "0:0", "dir"},
			WantOwners: map[string]owner{
				"dir":      {0, 0},
				"dir/link": {1000, 1000},
				"out":      {1000, 1000},
			},
		},
		{
			Name: "L follows the links of the walk",
			Files: fsx.MemFS{
				"dir":      dir(1000, 1000),
				"dir/link": link("../out", 1000, 1000),
				"out":      file(1000, 1000),
			},
			Args: []string{"-R", "-L", "0:0", "dir"},
			WantOwners: map[string]owner{
				"dir":      {0, 0},
				"dir/link": {1000, 1000},
				"out":      {0, 0},
			},
		},
		{
			Name: "L traverses a link to a directory of the walk",
			Files: fsx.MemFS{
				"dir":       dir(1000, 1000),
				"dir/sub":   dir(1000, 1000),
				"dir/sub/b": file(1000, 1000),
				"dir/link":  link("sub", 1000, 1000),
			},
			Args: []string{"-R", "-L", "0:0", "dir"},
			WantOwners: map[string]owner{
				"dir":       {0, 0},
				"dir/sub":   {0, 0},
				"dir/sub/b": {0, 0},
				"dir/link":  {1000, 1000},
			},
		},
		{
			Name:       "L reports a dangling link of the walk",
			Files:      fsx.MemFS{"dir": dir(1000, 1000), "dir/link": link("missing", 1000, 1000)},
			Args:       []string{"-R", "-L", "0:0", "dir"},
			WantOwners: map[string]owner{"dir": {0, 0}, "dir/link": {1000, 1000}},
			WantErr:    "chown: cannot dereference 'dir/link': file does not exist\n",
			WantExit:   1,
		},
		{
			Name: "L refuses a tree with a cyclic link",
			Files: fsx.MemFS{
				"d":    dir(1000, 1000),
				"d/f":  file(1000, 1000),
				"d/up": link(".", 1000, 1000),
			},
			Args:       []string{"-R", "-L", "0:0", "d"},
			WantOwners: map[string]owner{"d": {0, 0}, "d/f": {0, 0}},
			WantErr:    "chown: cycle in directory tree 'd/up'\n",
			WantExit:   1,
		},
		{
			Name:  "the last of H, L and P wins",
			Files: tree(),
			Args:  []string{"-R", "-L", "-H", "-P", "0:0", "dir"},
			WantOwners: map[string]owner{
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
			Name:  "h leaves the link itself alone",
			Files: linked(),
			Args:  []string{"-h", "0:0", "link"},
			WantOwners: map[string]owner{
				"file.txt": {1000, 1000},
				"link":     {1000, 1000},
			},
		},
		{
			Name:  "the default changes the referent",
			Files: linked(),
			Args:  []string{"0:0", "link"},
			WantOwners: map[string]owner{
				"file.txt": {0, 0},
				"link":     {1000, 1000},
			},
		},
		{
			Name:  "--dereference changes the referent",
			Files: linked(),
			Args:  []string{"--dereference", "0:0", "link"},
			WantOwners: map[string]owner{
				"file.txt": {0, 0},
				"link":     {1000, 1000},
			},
		},
		{
			Name:  "the last of -h and --dereference wins",
			Files: linked(),
			Args:  []string{"--dereference", "-h", "0:0", "link"},
			WantOwners: map[string]owner{
				"file.txt": {1000, 1000},
				"link":     {1000, 1000},
			},
		},
		{
			Name:  "the long form of -h works",
			Files: linked(),
			Args:  []string{"--no-dereference", "0:0", "link"},
			WantOwners: map[string]owner{
				"file.txt": {1000, 1000},
				"link":     {1000, 1000},
			},
		},
		{
			Name:       "h still changes a regular file",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-h", "0:0", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {0, 0}},
		},
		{
			Name:  "a chain of links is followed to the file",
			Files: fsx.MemFS{"file.txt": file(1000, 1000), "l2": link("file.txt", 1000, 1000), "l1": link("l2", 1000, 1000)},
			Args:  []string{"0:0", "l1"},
			WantOwners: map[string]owner{
				"file.txt": {0, 0},
				"l1":       {1000, 1000},
				"l2":       {1000, 1000},
			},
		},
		{
			Name:       "a dangling link is an error by default",
			Files:      fsx.MemFS{"dangling": link("missing", 1000, 1000)},
			Args:       []string{"0:0", "dangling"},
			WantOwners: map[string]owner{"dangling": {1000, 1000}},
			WantErr:    "chown: cannot dereference 'dangling': file does not exist\n",
			WantExit:   1,
		},
		{
			Name:       "a dangling link is left alone with -h",
			Files:      fsx.MemFS{"dangling": link("missing", 1000, 1000)},
			Args:       []string{"-h", "0:0", "dangling"},
			WantOwners: map[string]owner{"dangling": {1000, 1000}},
		},
	})
}

func TestRunVerbose(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:       "v reports a change of owner",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-v", "bob", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1001, 1000}},
			Want:       "changed ownership of 'file.txt' from alice to bob\n",
		},
		{
			Name:       "v reports a change of owner and group",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-v", "bob:devs", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1001, 1001}},
			Want:       "changed ownership of 'file.txt' from alice:staff to bob:devs\n",
		},
		{
			Name:       "v reports a change of group",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-v", ":devs", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1001}},
			Want:       "changed group of 'file.txt' from staff to devs\n",
		},
		{
			Name:       "v reports a retained owner",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-v", "alice", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			Want:       "ownership of 'file.txt' retained as alice\n",
		},
		{
			Name:       "v reports a retained group",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-v", ":staff", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			Want:       "group of 'file.txt' retained as staff\n",
		},
		{
			Name:       "v reports a lone colon as a retained ownership",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-v", ":", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			Want:       "ownership of 'file.txt' retained\n",
		},
		{
			Name:       "v reports unknown old IDs as numbers",
			Files:      fsx.MemFS{"file.txt": file(500, 500)},
			Args:       []string{"-v", "bob", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1001, 500}},
			Want:       "changed ownership of 'file.txt' from 500 to bob\n",
		},
		{
			Name:       "v reports a + prefixed owner as its number",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-v", "+1001", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1001, 1000}},
			Want:       "changed ownership of 'file.txt' from alice to 1001\n",
		},
		{
			Name:       "c reports only the files whose ownership changes",
			Files:      fsx.MemFS{"a.txt": file(1000, 1000), "b.txt": file(0, 1000)},
			Args:       []string{"-c", "0", "a.txt", "b.txt"},
			WantOwners: map[string]owner{"a.txt": {0, 1000}, "b.txt": {0, 1000}},
			Want:       "changed ownership of 'a.txt' from alice to 0\n",
		},
		{
			Name:       "c reports nothing when nothing changes",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-c", "alice", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
		},
		{
			Name:       "without c or v nothing is reported",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"bob", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1001, 1000}},
		},
		{
			Name:       "the last of c and v wins",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-c", "-v", "alice", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			Want:       "ownership of 'file.txt' retained as alice\n",
		},
		{
			Name:       "the last of v and c wins",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-v", "-c", "alice", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
		},
		{
			Name:       "the long options work",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"--verbose", "0", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {0, 1000}},
			Want:       "changed ownership of 'file.txt' from alice to 0\n",
		},
		{
			Name:       "v reports the operand as given",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-v", "bob", "./file.txt"},
			WantOwners: map[string]owner{"file.txt": {1001, 1000}},
			Want:       "changed ownership of './file.txt' from alice to bob\n",
		},
	})
}

func TestRunVerboseRecursive(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:  "v reports every file of the walk",
			Files: tree(),
			Args:  []string{"-Rv", "0:0", "dir"},
			WantOwners: map[string]owner{
				"dir":           {0, 0},
				"dir/a.txt":     {0, 0},
				"dir/sub":       {0, 0},
				"dir/sub/b.txt": {0, 0},
			},
			Want: "changed ownership of 'dir' from alice:staff to 0:0\n" +
				"changed ownership of 'dir/a.txt' from alice:staff to 0:0\n" +
				"changed ownership of 'dir/sub' from alice:staff to 0:0\n" +
				"changed ownership of 'dir/sub/b.txt' from alice:staff to 0:0\n",
		},
		{
			Name:  "c reports only the files whose ownership changes",
			Files: tree(),
			Args:  []string{"-Rc", "alice", "dir"},
			WantOwners: map[string]owner{
				"dir":           {1000, 1000},
				"dir/a.txt":     {1000, 1000},
				"dir/sub":       {1000, 1000},
				"dir/sub/b.txt": {1000, 1000},
			},
		},
		{
			Name:  "v doesn't report the links the walk leaves alone",
			Files: fsx.MemFS{"dir": dir(1000, 1000), "dir/link": link("out", 1000, 1000), "out": file(1000, 1000)},
			Args:  []string{"-Rv", "0:0", "dir"},
			WantOwners: map[string]owner{
				"dir":      {0, 0},
				"dir/link": {1000, 1000},
				"out":      {1000, 1000},
			},
			Want: "changed ownership of 'dir' from alice:staff to 0:0\n",
		},
	})
}

func TestRunFrom(t *testing.T) {
	two := func() fsx.MemFS {
		return fsx.MemFS{"a.txt": file(1000, 1000), "b.txt": file(1001, 1001)}
	}

	runTests(t, []testCase{
		{
			Name:       "from matches the owner",
			Files:      two(),
			Args:       []string{"--from=alice", "0:0", "a.txt", "b.txt"},
			WantOwners: map[string]owner{"a.txt": {0, 0}, "b.txt": {1001, 1001}},
		},
		{
			Name:       "from matches the owner and the group",
			Files:      two(),
			Args:       []string{"--from=alice:staff", "0:0", "a.txt", "b.txt"},
			WantOwners: map[string]owner{"a.txt": {0, 0}, "b.txt": {1001, 1001}},
		},
		{
			Name:       "from=OWNER: matches the owner across every group",
			Files:      fsx.MemFS{"a.txt": file(1000, 1000), "b.txt": file(1000, 1001), "c.txt": file(1001, 1000)},
			Args:       []string{"--from=alice:", "0:0", "a.txt", "b.txt", "c.txt"},
			WantOwners: map[string]owner{"a.txt": {0, 0}, "b.txt": {0, 0}, "c.txt": {1001, 1000}},
		},
		{
			Name:       "from=UID: matches a numeric owner across every group",
			Files:      fsx.MemFS{"a.txt": file(1000, 1000), "b.txt": file(1000, 1001)},
			Args:       []string{"--from=1000:", "0:0", "a.txt", "b.txt"},
			WantOwners: map[string]owner{"a.txt": {0, 0}, "b.txt": {0, 0}},
		},
		{
			Name:       "from=:GROUP matches the group across every owner",
			Files:      fsx.MemFS{"a.txt": file(1000, 1001), "b.txt": file(1001, 1001), "c.txt": file(1000, 1000)},
			Args:       []string{"--from=:devs", "0:0", "a.txt", "b.txt", "c.txt"},
			WantOwners: map[string]owner{"a.txt": {0, 0}, "b.txt": {0, 0}, "c.txt": {1000, 1000}},
		},
		{
			Name:       "from matches only the group",
			Files:      two(),
			Args:       []string{"--from=:staff", "0:0", "a.txt", "b.txt"},
			WantOwners: map[string]owner{"a.txt": {0, 0}, "b.txt": {1001, 1001}},
		},
		{
			Name:       "from does not match a different group",
			Files:      two(),
			Args:       []string{"--from=alice:devs", "0:0", "a.txt", "b.txt"},
			WantOwners: map[string]owner{"a.txt": {1000, 1000}, "b.txt": {1001, 1001}},
		},
		{
			Name:       "from does not match a different owner",
			Files:      two(),
			Args:       []string{"--from=bob", "0:0", "a.txt", "b.txt"},
			WantOwners: map[string]owner{"a.txt": {1000, 1000}, "b.txt": {0, 0}},
		},
		{
			Name:       "from matches a numeric ID",
			Files:      two(),
			Args:       []string{"--from=+1001", "0:0", "a.txt", "b.txt"},
			WantOwners: map[string]owner{"a.txt": {1000, 1000}, "b.txt": {0, 0}},
		},
		{
			Name:       "from works with R",
			Files:      fsx.MemFS{"d": dir(1000, 1000), "d/f": file(1001, 1001)},
			Args:       []string{"-R", "--from=alice", "0:0", "d"},
			WantOwners: map[string]owner{"d": {0, 0}, "d/f": {1001, 1001}},
		},
		{
			Name:       "v prints nothing for the files from skips",
			Files:      fsx.MemFS{"a.txt": file(1000, 1000)},
			Args:       []string{"-v", "--from=bob", "0:0", "a.txt"},
			WantOwners: map[string]owner{"a.txt": {1000, 1000}},
		},
		{
			Name:       "an invalid from is an error",
			Files:      fsx.MemFS{"a.txt": file(1000, 1000)},
			Args:       []string{"--from=nosuch", "0:0", "a.txt"},
			WantOwners: map[string]owner{"a.txt": {1000, 1000}},
			WantErr:    "chown: invalid user: \"nosuch\"\n",
			WantExit:   1,
		},
	})
}

func TestRunReference(t *testing.T) {
	ref := func() fsx.MemFS {
		return fsx.MemFS{"ref": file(1001, 1001), "file.txt": file(1000, 1000)}
	}

	runTests(t, []testCase{
		{
			Name:       "uses the ownership of the reference file",
			Files:      ref(),
			Args:       []string{"--reference=ref", "file.txt"},
			WantOwners: map[string]owner{"ref": {1001, 1001}, "file.txt": {1001, 1001}},
		},
		{
			Name:       "the reference can be a separate argument",
			Files:      ref(),
			Args:       []string{"--reference", "ref", "file.txt"},
			WantOwners: map[string]owner{"ref": {1001, 1001}, "file.txt": {1001, 1001}},
		},
		{
			Name:  "the ownership applies to every file",
			Files: fsx.MemFS{"ref": file(1001, 1001), "a.txt": file(0, 0), "b.txt": file(0, 0)},
			Args:  []string{"--reference=ref", "a.txt", "b.txt"},
			WantOwners: map[string]owner{
				"ref": {1001, 1001}, "a.txt": {1001, 1001}, "b.txt": {1001, 1001},
			},
		},
		{
			Name:  "a reference to a symbolic link is dereferenced",
			Files: fsx.MemFS{"ref": file(1001, 1001), "rlink": link("ref", 0, 0), "file.txt": file(1000, 1000)},
			Args:  []string{"--reference=rlink", "file.txt"},
			WantOwners: map[string]owner{
				"ref": {1001, 1001}, "rlink": {0, 0}, "file.txt": {1001, 1001},
			},
		},
		{
			Name:       "v reports the names of the reference",
			Files:      ref(),
			Args:       []string{"-v", "--reference=ref", "file.txt"},
			WantOwners: map[string]owner{"ref": {1001, 1001}, "file.txt": {1001, 1001}},
			Want:       "changed ownership of 'file.txt' from alice:staff to bob:devs\n",
		},
		{
			Name:       "a missing reference file is an error",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"--reference=missing", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: failed to get attributes of missing: file does not exist\n",
			WantExit:   1,
		},
		{
			Name:       "no operands is a usage error",
			Files:      fsx.MemFS{"ref": file(1001, 1001)},
			Args:       []string{"--reference=ref"},
			WantOwners: map[string]owner{"ref": {1001, 1001}},
			WantErr:    "chown: missing operand\nTry 'chown --help' for more information.\n",
			WantExit:   2,
		},
		{
			Name:  "every operand is a file with a reference",
			Files: fsx.MemFS{"ref": file(1001, 1001), "file.txt": file(1000, 1000)},
			Args:  []string{"--reference=ref", "1000", "file.txt"},
			WantOwners: map[string]owner{
				"ref": {1001, 1001}, "file.txt": {1001, 1001},
			},
			WantErr:  "chown: cannot access '1000': file does not exist\n",
			WantExit: 1,
		},
		{
			Name:  "a reference works with R",
			Files: fsx.MemFS{"ref": file(1001, 1001), "d": dir(1000, 1000), "d/f": file(1000, 1000)},
			Args:  []string{"-R", "--reference=ref", "d"},
			WantOwners: map[string]owner{
				"ref": {1001, 1001}, "d": {1001, 1001}, "d/f": {1001, 1001},
			},
		},
	})
}

func TestRunGeneral(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:       "a missing operand is an error",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       nil,
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: missing operand\nTry 'chown --help' for more information.\n",
			WantExit:   2,
		},
		{
			Name:       "a missing file operand is an error",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"alice"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: missing file operand\nTry 'chown --help' for more information.\n",
			WantExit:   2,
		},
		{
			Name:       "an invalid option is an error",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-x", "alice", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: invalid option -- 'x'\nTry 'chown --help' for more information.\n",
			WantExit:   2,
		},
		{
			Name:       "an invalid option in a group is an error",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-Rx", "alice", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: invalid option -- 'x'\nTry 'chown --help' for more information.\n",
			WantExit:   2,
		},
		{
			Name:       "an unrecognized long option",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"--bogus", "alice", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: unrecognized option '--bogus'\nTry 'chown --help' for more information.\n",
			WantExit:   2,
		},
		{
			Name:       "a missing option argument is an error",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"--from"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: option '--from' requires an argument\nTry 'chown --help' for more information.\n",
			WantExit:   2,
		},
		{
			Name:       "an owner starting with a dash can follow a double dash",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"--", "-42", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: invalid user: \"-42\"\n",
			WantExit:   1,
		},
		{
			Name:       "a file named like an option can follow a double dash",
			Files:      fsx.MemFS{"-w": file(1000, 1000)},
			Args:       []string{"--", "0", "-w"},
			WantOwners: map[string]owner{"-w": {0, 1000}},
		},
		{
			Name:  "options can be grouped",
			Files: tree(),
			Args:  []string{"-Rv", "0:0", "dir"},
			WantOwners: map[string]owner{
				"dir":           {0, 0},
				"dir/a.txt":     {0, 0},
				"dir/sub":       {0, 0},
				"dir/sub/b.txt": {0, 0},
			},
			Want: "changed ownership of 'dir' from alice:staff to 0:0\n" +
				"changed ownership of 'dir/a.txt' from alice:staff to 0:0\n" +
				"changed ownership of 'dir/sub' from alice:staff to 0:0\n" +
				"changed ownership of 'dir/sub/b.txt' from alice:staff to 0:0\n",
		},
		{
			Name:       "a missing file is an error, but the others are changed",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"0", "missing", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {0, 1000}},
			WantErr:    "chown: cannot access 'missing': file does not exist\n",
			WantExit:   1,
		},
		{
			Name:       "f suppresses the diagnostics but keeps the status",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-f", "0", "missing", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {0, 1000}},
			WantExit:   1,
		},
	})
}

func TestRunPreserveRoot(t *testing.T) {
	failsafe := "chown: it is dangerous to operate recursively on '/'\n" +
		"chown: use --no-preserve-root to override this failsafe\n"

	runTests(t, []testCase{
		{
			Name:       "R on the root is not refused by default",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-R", "0:0", "/"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: cannot access '/': invalid argument\n",
			WantExit:   1,
		},
		{
			Name:       "preserve-root refuses a recursive run on /",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"--preserve-root", "-R", "0:0", "/"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    failsafe,
			WantExit:   1,
		},
		{
			Name:       "preserve-root also catches a cleaned / operand",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-R", "--preserve-root", "0:0", "/usr/.."},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    failsafe,
			WantExit:   1,
		},
		{
			Name:       "no-preserve-root overrides the failsafe",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"--preserve-root", "--no-preserve-root", "-R", "0:0", "/"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: cannot access '/': invalid argument\n",
			WantExit:   1,
		},
		{
			Name:       "the last of the two options wins",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"--no-preserve-root", "--preserve-root", "-R", "0:0", "/"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    failsafe,
			WantExit:   1,
		},
		{
			Name:       "without R the option has no effect",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"--preserve-root", "0:0", "/"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: cannot access '/': invalid argument\n",
			WantExit:   1,
		},
		{
			Name:       "preserve-root leaves the other hierarchies alone",
			Files:      fsx.MemFS{"d": dir(1000, 1000), "d/f": file(1000, 1000)},
			Args:       []string{"--preserve-root", "-R", "0:0", "d"},
			WantOwners: map[string]owner{"d": {0, 0}, "d/f": {0, 0}},
		},
		{
			Name:       "preserve-root leaves a followed link to a hierarchy alone",
			Files:      fsx.MemFS{"dir": dir(1000, 1000), "dlink": link("dir", 1000, 1000)},
			Args:       []string{"--preserve-root", "-R", "-H", "0:0", "dlink"},
			WantOwners: map[string]owner{"dir": {0, 0}, "dlink": {1000, 1000}},
		},
		{
			Name:       "preserve-root refuses a link operand pointing at / with H",
			Files:      fsx.MemFS{"rlink": link("/", 1000, 1000), "file.txt": file(1000, 1000)},
			Args:       []string{"--preserve-root", "-R", "-H", "0:0", "rlink"},
			WantOwners: map[string]owner{"rlink": {1000, 1000}, "file.txt": {1000, 1000}},
			WantErr:    failsafe,
			WantExit:   1,
		},
		{
			Name:       "preserve-root refuses a link operand pointing at / with L",
			Files:      fsx.MemFS{"rlink": link("/", 1000, 1000), "file.txt": file(1000, 1000)},
			Args:       []string{"--preserve-root", "-R", "-L", "0:0", "rlink"},
			WantOwners: map[string]owner{"rlink": {1000, 1000}, "file.txt": {1000, 1000}},
			WantErr:    failsafe,
			WantExit:   1,
		},
		{
			Name:       "preserve-root refuses a chain of links pointing at /",
			Files:      fsx.MemFS{"l2": link("/", 1000, 1000), "l1": link("l2", 1000, 1000)},
			Args:       []string{"--preserve-root", "-R", "-L", "0:0", "l1"},
			WantOwners: map[string]owner{"l1": {1000, 1000}, "l2": {1000, 1000}},
			WantErr:    failsafe,
			WantExit:   1,
		},
		{
			Name:       "a link operand pointing at / is not refused by default",
			Files:      fsx.MemFS{"rlink": link("/", 1000, 1000), "file.txt": file(1000, 1000)},
			Args:       []string{"-R", "-H", "0:0", "rlink"},
			WantOwners: map[string]owner{"file.txt": {0, 0}, "rlink": {1000, 1000}},
		},
	})
}

func TestRunLongOptions(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:       "recursive changes every file below the directory",
			Files:      fsx.MemFS{"d": dir(1000, 1000), "d/f": file(1000, 1000)},
			Args:       []string{"--recursive", "0:0", "d"},
			WantOwners: map[string]owner{"d": {0, 0}, "d/f": {0, 0}},
		},
		{
			Name:       "silent suppresses the diagnostics",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"--silent", "0", "missing", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {0, 1000}},
			WantExit:   1,
		},
		{
			Name:       "quiet suppresses the diagnostics too",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"--quiet", "0", "missing", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {0, 1000}},
			WantExit:   1,
		},
	})
}

func TestRunConflict(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:       "R with dereference needs a traversal mode",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-R", "--dereference", "0:0", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: -R --dereference requires either -H or -L\n",
			WantExit:   1,
		},
		{
			Name:       "R with -P and dereference is refused too",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"-R", "-P", "--dereference", "0:0", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: -R --dereference requires either -H or -L\n",
			WantExit:   1,
		},
		{
			Name:       "the option order doesn't matter for the check",
			Files:      fsx.MemFS{"file.txt": file(1000, 1000)},
			Args:       []string{"--dereference", "-R", "0:0", "file.txt"},
			WantOwners: map[string]owner{"file.txt": {1000, 1000}},
			WantErr:    "chown: -R --dereference requires either -H or -L\n",
			WantExit:   1,
		},
		{
			Name:  "R with dereference and H dereferences the walk",
			Files: fsx.MemFS{"d": dir(1000, 1000), "d/link": link("../out", 1000, 1000), "out": file(1000, 1000)},
			Args:  []string{"-R", "--dereference", "-H", "0:0", "d"},
			WantOwners: map[string]owner{
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
		if code := testutil.ExitCode(t, err); code != 1 {
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
		if code := testutil.ExitCode(t, err); code != 1 {
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
		if code := testutil.ExitCode(t, err); code != 1 {
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
		if code := testutil.ExitCode(t, err); code != 1 {
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
	if code := testutil.ExitCode(t, err); code != 1 {
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
	} else if code := testutil.ExitCode(t, err); code != 1 {
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
	if code := testutil.ExitCode(t, err); code != 1 {
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
			Files:      files,
			Args:       []string{"1000:1000", "sub\\file.txt"},
			WantOwners: map[string]owner{"sub/file.txt": {1000, 1000}},
		})
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "chown", usage)
}
