package find

import (
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"charm.land/turtle/fsx"
)

func TestRunDepthLimits(t *testing.T) {
	deep := fsx.MemoryFS{"dir/d1/d2/d3/d4/d5/d6": {}}
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemoryFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name: "maxdepth 1",
			args: []string{"dir", "-maxdepth", "1"},
			files: fsx.MemoryFS{
				"dir/d1/d2/d3/d4/d5/d6": {},
			},
			want: "dir\ndir/d1\n",
		},
		{
			name:  "maxdepth 0 applies to the paths alone",
			args:  []string{"dir", "-maxdepth", "0"},
			files: fsx.MemoryFS{"dir/d1": {}},
			want:  "dir\n",
		},
		{
			name:  "mindepth 5",
			args:  []string{"dir", "-mindepth", "5"},
			files: deep,
			want:  "dir/d1/d2/d3/d4/d5\ndir/d1/d2/d3/d4/d5/d6\n",
		},
		{
			name:  "mindepth 1 skips the paths",
			args:  []string{"dir", "-mindepth", "1"},
			files: fsx.MemoryFS{"dir/d1/d2": {}},
			want:  "dir/d1\ndir/d1/d2\n",
		},
		{
			name:  "mindepth and maxdepth",
			args:  []string{"dir", "-mindepth", "2", "-maxdepth", "4"},
			files: deep,
			want:  "dir/d1/d2\ndir/d1/d2/d3\ndir/d1/d2/d3/d4\n",
		},
		{
			name:     "invalid maxdepth",
			args:     []string{"dir", "-maxdepth", "x"},
			wantErr:  "find: invalid argument 'x' to -maxdepth\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunGNUOperators(t *testing.T) {
	files := fsx.MemoryFS{"a": {}, "b": {}, "c": {}}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "true and false",
			args: []string{".", "-true", "-print"},
			want: ".\n./a\n./b\n./c\n",
		},
		{
			name: "false never acts",
			args: []string{".", "-false", "-print"},
		},
		{
			name: "and",
			args: []string{".", "-name", "a*", "-and", "-print"},
			want: "./a\n",
		},
		{
			name: "or",
			args: []string{".", "-name", "a", "-or", "-name", "b"},
			want: "./a\n./b\n",
		},
		{
			name: "not",
			args: []string{".", "-not", "-name", "a", "-print"},
			want: ".\n./b\n./c\n",
		},
		{
			name: "the comma operator evaluates both sides",
			args: []string{".", "-name", "a", "-print", ",", "-name", "b", "-print"},
			want: "./a\n./b\n",
		},
		{
			name: "the comma operator keeps the value of the right side",
			args: []string{".", "-name", "a", ",", "-name", "b", "-print"},
			want: "./b\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, files, nil, nil, test.want, "", 0)
		})
	}
}

func TestRunNameVariants(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemoryFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "path",
			args:  []string{".", "-path", "*/a"},
			files: fsx.MemoryFS{"dir/a": {}, "dir/sub/a": {}},
			want:  "./dir/a\n./dir/sub/a\n",
		},
		{
			name:  "wholename is path",
			args:  []string{".", "-wholename", "*/a"},
			files: fsx.MemoryFS{"dir/a": {}},
			want:  "./dir/a\n",
		},
		{
			name:  "ipath ignores case",
			args:  []string{".", "-ipath", "*/A"},
			files: fsx.MemoryFS{"dir/a": {}},
			want:  "./dir/a\n",
		},
		{
			name:  "iwholename ignores case",
			args:  []string{".", "-iwholename", "*/A"},
			files: fsx.MemoryFS{"dir/a": {}},
			want:  "./dir/a\n",
		},
		{
			name:  "path matches the name as given",
			args:  []string{"dir", "-path", "dir/a"},
			files: fsx.MemoryFS{"dir/a": {}},
			want:  "dir/a\n",
		},
		{
			name:  "path with another starting point doesn't match",
			args:  []string{"dir", "-path", "./dir/a"},
			files: fsx.MemoryFS{"dir/a": {}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunRegex(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemoryFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "extended by default",
			args:  []string{".", "-regex", `\./(a|b)`},
			files: fsx.MemoryFS{"a": {}, "b": {}, "ab": {}},
			want:  "./a\n./b\n",
		},
		{
			name:  "the whole path must match",
			args:  []string{".", "-regex", `\./a`},
			files: fsx.MemoryFS{"a": {}, "ba": {}},
			want:  "./a\n",
		},
		{
			name:  "iregex ignores case",
			args:  []string{".", "-iregex", `\./A`},
			files: fsx.MemoryFS{"a": {}},
			want:  "./a\n",
		},
		{
			name:  "regextype selects basic expressions",
			args:  []string{".", "-regextype", "posix-basic", "-regex", `\./a`},
			files: fsx.MemoryFS{"a": {}, "b": {}},
			want:  "./a\n",
		},
		{
			name:     "unknown regextype",
			args:     []string{".", "-regextype", "frob"},
			wantErr:  "find: unknown regular expression type 'frob'\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "invalid expression",
			args:     []string{".", "-regex", "("},
			wantErr:  "find: invalid regular expression '('\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunLname(t *testing.T) {
	files := symlinkFS(t, "ln", "dir/target", fsx.MemoryFS{"dir/target": {}})
	tests := []struct {
		name     string
		args     []string
		want     string
		wantCode int
	}{
		{
			name: "the target pattern matches",
			args: []string{"ln", "-lname", "dir/*"},
			want: "ln\n",
		},
		{
			name: "no match",
			args: []string{"ln", "-lname", "other/*"},
		},
		{
			name: "ilname ignores case",
			args: []string{"ln", "-ilname", "DIR/*"},
			want: "ln\n",
		},
		{
			name: "not a symbolic link",
			args: []string{"dir/target", "-lname", "*"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, files, nil, nil, test.want, "", test.wantCode)
		})
	}
}

func TestRunTypeLists(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemoryFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "comma separated types",
			args:  []string{".", "-type", "f,d"},
			files: fsx.MemoryFS{"a": {}, "d/b": {}},
			want:  ".\n./a\n./d\n./d/b\n",
		},
		{
			name:  "three types",
			args:  []string{".", "-type", "f,d,l", "-print"},
			files: symlinkFS(t, "ln", "a", fsx.MemoryFS{"a": {}, "d/b": {}}),
			want:  ".\n./a\n./d\n./d/b\n./ln\n",
		},
		{
			name:     "unknown type",
			args:     []string{".", "-type", "x"},
			wantErr:  "find: unknown type 'x'\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunXType(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemoryFS
		want     string
		wantCode int
	}{
		{
			name:  "the type of the target",
			args:  []string{"ln", "-xtype", "f"},
			files: symlinkFS(t, "ln", "a", fsx.MemoryFS{"a": {}}),
			want:  "ln\n",
		},
		{
			name:  "not the type of the link",
			args:  []string{"ln", "-xtype", "l"},
			files: symlinkFS(t, "ln", "a", fsx.MemoryFS{"a": {}}),
		},
		{
			name:  "a broken link is of type l",
			args:  []string{"ln", "-xtype", "l"},
			files: symlinkFS(t, "ln", "missing", fsx.MemoryFS{}),
			want:  "ln\n",
		},
		{
			name:  "a broken link is not of the type of the target",
			args:  []string{"ln", "-xtype", "f"},
			files: symlinkFS(t, "ln", "missing", fsx.MemoryFS{}),
		},
		{
			name:  "a followed link is of the type of its target",
			args:  []string{"-L", "ln", "-xtype", "f"},
			files: symlinkFS(t, "ln", "a", fsx.MemoryFS{"a": {}}),
			want:  "ln\n",
		},
		{
			name:  "a non-link behaves like -type",
			args:  []string{"a", "-xtype", "f"},
			files: fsx.MemoryFS{"a": {}},
			want:  "a\n",
		},
		{
			name:  "a broken link is of type l under -L",
			args:  []string{"-L", "ln", "-xtype", "l"},
			files: symlinkFS(t, "ln", "missing", fsx.MemoryFS{}),
			want:  "ln\n",
		},
		{
			name:  "a broken link is not of another type under -L",
			args:  []string{"-L", "ln", "-xtype", "f"},
			files: symlinkFS(t, "ln", "missing", fsx.MemoryFS{}),
		},
		{
			name:  "a followed link is of the type of its target",
			args:  []string{"-L", "ln", "-xtype", "f"},
			files: symlinkFS(t, "ln", "a", fsx.MemoryFS{"a": {}}),
			want:  "ln\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, "", test.wantCode)
		})
	}
}

func TestRunPermForms(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemoryFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "exact mode",
			args:  []string{".", "-perm", "644"},
			files: fsx.MemoryFS{"a": {Mode: 0o644}, "b": {Mode: 0o755}},
			want:  "./a\n",
		},
		{
			name:  "all of the bits",
			args:  []string{".", "-perm", "-644"},
			files: fsx.MemoryFS{"a": {Mode: 0o777}, "b": {Mode: 0o600}},
			want:  ".\n./a\n",
		},
		{
			name:  "any of the bits",
			args:  []string{".", "-perm", "/222"},
			files: fsx.MemoryFS{"a": {Mode: 0o644}, "b": {Mode: 0o444}},
			want:  ".\n./a\n",
		},
		{
			name:  "any with no bits matches every file",
			args:  []string{".", "-perm", "/000"},
			files: fsx.MemoryFS{"a": {Mode: 0o644}, "b": {Mode: 0o444}},
			want:  ".\n./a\n./b\n",
		},
		{
			name:  "symbolic any",
			args:  []string{"a", "-perm", "/g+w,o+w"},
			files: fsx.MemoryFS{"a": {Mode: 0o646}},
			want:  "a\n",
		},
		{
			name:  "symbolic all",
			args:  []string{"a", "-perm", "-g+w,o+w"},
			files: fsx.MemoryFS{"a": {Mode: 0o666}},
			want:  "a\n",
		},
		{
			name:  "the deprecated plus form of the slash",
			args:  []string{"a", "-perm", "+g+w"},
			files: fsx.MemoryFS{"a": {Mode: 0o620}},
			want:  "a\n",
		},
		{
			name:     "plus before an octal digit",
			args:     []string{".", "-perm", "+644"},
			wantErr:  "find: invalid mode '+644': mode must not start with '+' followed by an octal digit\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "invalid mode",
			args:     []string{".", "-perm", "9999"},
			wantErr:  "find: invalid mode '9999'\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunGNUIds(t *testing.T) {
	files := fsx.MemoryFS{
		"a": {Uid: 424242, Gid: 424243},
		"b": {Uid: 0, Gid: 0},
	}
	tests := []struct {
		name     string
		args     []string
		want     string
		wantCode int
	}{
		{
			name: "uid",
			args: []string{".", "-uid", "424242"},
			want: "./a\n",
		},
		{
			name: "uid greater than",
			args: []string{".", "-uid", "+0"},
			want: "./a\n",
		},
		{
			name: "gid",
			args: []string{".", "-gid", "424243"},
			want: "./a\n",
		},
		{
			name: "inum on a file system without inode numbers",
			args: []string{".", "-inum", "0"},
			want: ".\n./a\n./b\n",
		},
		{
			name: "inum with a number",
			args: []string{".", "-inum", "1"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, files, nil, nil, test.want, "", test.wantCode)
		})
	}
}

func TestRunSizeUnits(t *testing.T) {
	files := fsx.MemoryFS{
		"a":   {Data: []byte("123456789")},
		"b":   {Data: []byte("12345")},
		"e":   {},
		"big": {Data: make([]byte, 2*1024*1024)},
	}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "default unit is 512 bytes",
			args: []string{"a", "b", "e", "-size", "1"},
			want: "a\nb\n",
		},
		{
			name: "a small file is a single block",
			args: []string{"b", "-size", "1"},
			want: "b\n",
		},
		{
			name: "a big file is many blocks",
			args: []string{"big", "-size", "4096"},
			want: "big\n",
		},
		{
			name: "bytes",
			args: []string{"a", "-size", "9c"},
			want: "a\n",
		},
		{
			name: "words",
			args: []string{"a", "b", "e", "-size", "3w"},
			want: "b\n",
		},
		{
			name: "kibibytes",
			args: []string{"big", "-size", "2048k"},
			want: "big\n",
		},
		{
			name: "mebibytes",
			args: []string{"big", "-size", "2M"},
			want: "big\n",
		},
		{
			name: "less than a unit, not an exact one",
			args: []string{"a", "b", "e", "-size", "-1k"},
			want: "e\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, files, nil, nil, test.want, "", 0)
		})
	}
}

func TestRunMinutes(t *testing.T) {
	files := fsx.MemoryFS{
		"a": {ModTime: testNow.Add(-3 * time.Minute)},
		"b": {ModTime: testNow.Add(-10 * time.Minute)},
	}
	tests := []struct {
		name     string
		args     []string
		want     string
		wantCode int
	}{
		{
			name: "mmin",
			args: []string{".", "-mmin", "3"},
			want: "./a\n",
		},
		{
			name: "a range of minutes",
			args: []string{".", "-mmin", "+2", "-mmin", "-6"},
			want: "./a\n",
		},
		{
			name: "amin falls back to the modification time",
			args: []string{".", "-amin", "10"},
			want: "./b\n",
		},
		{
			name: "cmin falls back to the modification time",
			args: []string{".", "-cmin", "10"},
			want: "./b\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, files, nil, nil, test.want, "", test.wantCode)
		})
	}
}

func TestRunDaystart(t *testing.T) {
	// The times are measured from the beginning of the day, at local
	// midnight, which the test computes the same way.
	midnight := testNow.Local()
	midnight = time.Date(midnight.Year(), midnight.Month(), midnight.Day(), 0, 0, 0, 0, midnight.Location())
	files := fsx.MemoryFS{"a": {ModTime: midnight.Add(-48 * time.Hour)}}
	runTest(t, []string{"a", "-daystart", "-mtime", "2"}, files, nil, nil, "a\n", "", 0)
	runTest(t, []string{"a", "-daystart", "-mtime", "+2"}, files, nil, nil, "", "", 0)
	files["b"] = &fsx.MemoryFile{ModTime: midnight.Add(-96 * time.Hour)}
	runTest(t, []string{"b", "-daystart", "-mtime", "4"}, files, nil, nil, "b\n", "", 0)
	runTest(t, []string{"b", "-mtime", "+2"}, files, nil, nil, "b\n", "", 0)
}

func TestRunNewerXY(t *testing.T) {
	files := fsx.MemoryFS{
		"ref": {ModTime: testNow.Add(-48 * time.Hour)},
		"a":   {ModTime: testNow.Add(-24 * time.Hour)},
		"b":   {ModTime: testNow.Add(-72 * time.Hour)},
	}
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemoryFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "neweram",
			args:  []string{".", "-neweram", "ref"},
			files: files,
			want:  "./a\n",
		},
		{
			name:  "newercm",
			args:  []string{".", "-newercm", "ref"},
			files: files,
			want:  "./a\n",
		},
		{
			name:  "newermm is newer",
			args:  []string{".", "-newermm", "ref"},
			files: files,
			want:  "./a\n",
		},
		{
			name:  "newerct takes a date string",
			args:  []string{".", "-newerct", "2025-12-30"},
			files: files,
			want:  "./a\n./ref\n",
		},
		{
			name:  "newerat takes a date string",
			args:  []string{".", "-newerat", "2025-12-30"},
			files: files,
			want:  "./a\n./ref\n",
		},
		{
			name:  "nothing newer than a late date",
			args:  []string{".", "-newermt", "2027-01-01"},
			files: files,
			want:  "",
		},
		{
			name:     "an invalid date",
			args:     []string{".", "-newermt", "frob"},
			files:    files,
			wantErr:  "find: invalid date 'frob'\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "t as the file time",
			args:     []string{".", "-newertm", "2026-01-01"},
			files:    files,
			wantErr:  "find: invalid argument -newertm\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:  "the birth time of the file isn't available",
			args:  []string{".", "-newerBm", "ref"},
			files: files,
		},
		{
			name:     "the birth time of the reference isn't available",
			args:     []string{".", "-newerBB", "ref"},
			files:    files,
			wantErr:  "find: cannot use the birth time of 'ref': it is not available\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunUsed(t *testing.T) {
	// A memory file reports no change time, so both times fall back to the
	// modification one, which the access time also starts as.
	files := fsx.MemoryFS{"a": {ModTime: testNow.Add(-48 * time.Hour)}}
	runTest(t, []string{".", "-used", "0"}, files, nil, nil, ".\n./a\n", "", 0)
	runTest(t, []string{".", "-used", "+1"}, files, nil, nil, "", "", 0)
}

func TestRunEmpty(t *testing.T) {
	files := fsx.MemoryFS{
		"e": {},
		"f": {Data: []byte("x")},
		"d": {Mode: fs.ModeDir},
	}
	runTest(t, []string{".", "-empty"}, files, nil, nil, "./d\n./e\n", "", 0)
}

func TestRunAccess(t *testing.T) {
	// The bits of the others are used, so the checks hold whichever user
	// runs the tests, superuser excepted.
	files := fsx.MemoryFS{
		"r":  {Mode: 0o004},
		"rw": {Mode: 0o002},
		"x":  {Mode: 0o001},
		"n":  {Mode: 0o000},
	}
	runTest(t, []string{"r", "rw", "x", "n", "-readable"}, files, nil, nil, "r\n", "", 0)
	runTest(t, []string{"r", "rw", "x", "n", "-executable"}, files, nil, nil, "x\n", "", 0)
	runTest(t, []string{"r", "rw", "x", "n", "-writable"}, files, nil, nil, "rw\n", "", 0)
}

func TestRunFollow(t *testing.T) {
	files := symlinkFS(t, "ln", "d", fsx.MemoryFS{"d/a": {}})
	runTest(t, []string{"ln", "-follow"}, files, nil, nil, "ln\nln/a\n", "", 0)
	// Without -follow the link is not descended into.
	runTest(t, []string{"ln", "-name", "*"}, files, nil, nil, "ln\n", "", 0)
}

func TestRunQuit(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		files fsx.MemoryFS
		want  string
	}{
		{
			name:  "the traversal stops at the first file",
			args:  []string{".", "-print", "-quit"},
			files: fsx.MemoryFS{"a": {}},
			want:  ".\n",
		},
		{
			name:  "no starting point after the quit one is processed",
			args:  []string{"d1", "d2", "-print", "-quit"},
			files: fsx.MemoryFS{"d1/a": {}, "d2/a": {}},
			want:  "d1\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, "", 0)
		})
	}
}

func TestRunFiles0From(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		stdin    io.Reader
		files    fsx.MemoryFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "starting points from the standard input",
			args:  []string{"-files0-from", "-", "-maxdepth", "0"},
			stdin: strings.NewReader("dir/a\x00dir/b\x00"),
			files: fsx.MemoryFS{"dir/a": {}, "dir/b": {}},
			want:  "dir/a\ndir/b\n",
		},
		{
			name:  "starting points from a file",
			args:  []string{"-files0-from", "list"},
			files: fsx.MemoryFS{"list": {Data: []byte("a\x00b\x00")}, "a": {}, "b": {}},
			want:  "a\nb\n",
		},
		{
			name:  "an empty list processes nothing",
			args:  []string{"-files0-from", "list"},
			files: fsx.MemoryFS{"list": {}, "a": {}},
			want:  "",
		},
		{
			name:     "an empty name is an error",
			args:     []string{"-files0-from", "-"},
			stdin:    strings.NewReader("a\x00\x00"),
			wantErr:  "find: zero-length file name in -files0-from input\n",
			wantCode: 1,
		},
		{
			name:     "combined with a starting point",
			args:     []string{"-files0-from", "list", "a"},
			files:    fsx.MemoryFS{"list": {Data: []byte("a\x00")}, "a": {}},
			wantErr:  "find: the -files0-from option cannot be combined with a starting point\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "a missing list",
			args:     []string{"-files0-from", "missing"},
			wantErr:  "find: cannot open 'missing': file does not exist\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, test.stdin, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunOptDebug(t *testing.T) {
	files := fsx.MemoryFS{"a": {}}
	runTest(t, []string{"-O3", ".", "-print"}, files, nil, nil, ".\n./a\n", "", 0)
	runTest(t, []string{"-O", "1", ".", "-print"}, files, nil, nil, ".\n./a\n", "", 0)
	runTest(t, []string{"-D", "tree", ".", "-print"}, files, nil, nil, ".\n./a\n", "", 0)
	runTest(t, []string{"-D", "Help,Tree", ".", "-print"}, files, nil, nil, ".\n./a\n", "", 0)
	runTest(t, []string{"-D", "frob", ".", "-print"}, files, nil, nil, "",
		"find: unknown debug option 'frob'\nTry 'find --help' for more information.\n", 1)
	runTest(t, []string{"-Ox", ".", "-print"}, files, nil, nil, "",
		"find: invalid optimisation level 'x'\nTry 'find --help' for more information.\n", 1)
}

func TestRunUnsupportedPrimaries(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "fstype",
			args:    []string{".", "-fstype", "apfs"},
			wantErr: "find: -fstype is not supported\nTry 'find --help' for more information.\n",
		},
		{
			name:    "context",
			args:    []string{".", "-context", "x"},
			wantErr: "find: -context is not supported\nTry 'find --help' for more information.\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, fsx.MemoryFS{}, nil, nil, "", test.wantErr, 1)
		})
	}
}
