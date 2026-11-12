package install

import (
	"context"
	"errors"
	"io/fs"
	"runtime"
	"strings"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/fsop"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

var (
	oldTime   = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	otherTime = time.Date(1999, 12, 31, 23, 59, 58, 0, time.UTC)
)

// link is a MemFS entry holding a symbolic link to target.
func link(target string) *fsx.MemFile {
	return &fsx.MemFile{Mode: fs.ModeSymlink, Data: []byte(target)}
}

func TestRunCopy(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "two operands copy a file",
			FS:        fsx.MemFS{"a.txt": {Data: []byte("x")}},
			Args:      []string{"a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "x"},
			WantMode:  map[string]fs.FileMode{"b.txt": 0o755},
		},
		{
			Name:      "multiple sources copy into a directory",
			FS:        fsx.MemFS{"a": {Data: []byte("1")}, "b": {Data: []byte("2")}, "d": {Mode: fs.ModeDir | 0o755}},
			Args:      []string{"a", "b", "d"},
			WantFiles: map[string]string{"d/a": "1", "d/b": "2"},
			WantMode:  map[string]fs.FileMode{"d/a": 0o755, "d/b": 0o755},
		},
		{
			Name:      "-t copies into a directory",
			FS:        fsx.MemFS{"a": {Data: []byte("1")}, "d": {Mode: fs.ModeDir | 0o755}},
			Args:      []string{"-t", "d", "a"},
			WantFiles: map[string]string{"d/a": "1"},
		},
		{
			Name:     "-T refuses a directory destination",
			FS:       fsx.MemFS{"a": {Data: []byte("1")}, "d": {Mode: fs.ModeDir | 0o755}},
			Args:     []string{"-T", "a", "d"},
			WantErr:  "install: cannot overwrite directory 'd' with non-directory 'a'\n",
			WantExit: 1,
		},
		{
			Name:      "an existing destination is replaced",
			FS:        fsx.MemFS{"a": {Data: []byte("new"), Mode: 0o600}, "b": {Data: []byte("old"), Mode: 0o600}},
			Args:      []string{"a", "b"},
			WantFiles: map[string]string{"b": "new"},
			WantMode: map[string]fs.FileMode{
				"b": 0o755,
			},
		},
		{
			Name:     "the source mode is not used",
			FS:       fsx.MemFS{"a": {Data: []byte("x"), Mode: 0o600}},
			Args:     []string{"a", "b"},
			WantMode: map[string]fs.FileMode{"b": 0o755},
		},
		{
			Name:      "the umask never masks the mode",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}},
			Umask:     0o077,
			Args:      []string{"a", "b"},
			WantFiles: map[string]string{"b": "x"},
			WantMode:  map[string]fs.FileMode{"b": 0o755},
		},
		{
			Name:     "a missing source is an error",
			FS:       fsx.MemFS{},
			Args:     []string{"missing.txt", "b.txt"},
			Absent:   []string{"b.txt"},
			WantErr:  "install: cannot stat 'missing.txt': file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing destination parent is an error",
			FS:       fsx.MemFS{"a.txt": {Data: []byte("x")}},
			Args:     []string{"a.txt", "nodir/b.txt"},
			Absent:   []string{"nodir"},
			WantErr:  "install: cannot create 'nodir/b.txt': file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a directory source is omitted",
			FS:       fsx.MemFS{"d": {Mode: fs.ModeDir | 0o755}},
			Args:     []string{"d", "b"},
			Absent:   []string{"b"},
			WantErr:  "install: omitting directory 'd'\n",
			WantExit: 1,
		},
		{
			Name:     "a file is not copied onto itself",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}},
			Args:     []string{"a", "a"},
			WantErr:  "install: 'a' and 'a' are the same file\n",
			WantExit: 1,
		},
		{
			Name:     "a file is not copied onto itself through a link",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}, "l": link("a")},
			Args:     []string{"a", "l"},
			WantErr:  "install: 'a' and 'l' are the same file\n",
			WantExit: 1,
		},
		{
			Name:     "a directory destination is not overwritten",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}, "d": {Mode: fs.ModeDir | 0o755}},
			Args:     []string{"-T", "a", "d"},
			WantErr:  "install: cannot overwrite directory 'd' with non-directory 'a'\n",
			WantExit: 1,
		},
		{
			Name:     "no operand is a usage error",
			FS:       fsx.MemFS{},
			Args:     nil,
			WantErr:  "install: missing file operand\nTry 'install --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "one operand misses the destination",
			FS:       fsx.MemFS{"a": {}},
			Args:     []string{"a"},
			WantErr:  "install: missing destination file operand after 'a'\nTry 'install --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an extra operand is a usage error with -T",
			FS:       fsx.MemFS{"a": {}, "b": {}, "c": {}},
			Args:     []string{"-T", "a", "b", "c"},
			WantErr:  "install: extra operand 'c'\nTry 'install --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "several sources need a directory target",
			FS:       fsx.MemFS{"a": {Data: []byte("1")}, "b": {Data: []byte("2")}, "c": {Data: []byte("3")}},
			Args:     []string{"a", "b", "c"},
			WantErr:  "install: target 'c' is not a directory\n",
			WantExit: 1,
		},
		{
			Name:     "-t needs a directory",
			FS:       fsx.MemFS{"a": {Data: []byte("1")}},
			Args:     []string{"-t", "nodir", "a"},
			WantErr:  "install: failed to access 'nodir': file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "-t refuses a file target",
			FS:       fsx.MemFS{"a": {Data: []byte("1")}, "f": {Data: []byte("2")}},
			Args:     []string{"-t", "f", "a"},
			WantErr:  "install: failed to access 'f': not a directory\n",
			WantExit: 1,
		},
		{
			Name:     "-t can't be combined with -T",
			FS:       fsx.MemFS{},
			Args:     []string{"-t", "d", "-T"},
			WantErr:  "install: cannot combine --target-directory (-t) and --no-target-directory (-T)\nTry 'install --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a target directory can't be given twice",
			FS:       fsx.MemFS{"d": {Mode: fs.ModeDir | 0o755}},
			Args:     []string{"-t", "d", "-t", "d"},
			WantErr:  "install: multiple target directories specified\nTry 'install --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "-d can't take a target directory",
			FS:       fsx.MemFS{"d": {Mode: fs.ModeDir | 0o755}},
			Args:     []string{"-d", "-t", "d"},
			WantErr:  "install: target directory not allowed when installing a directory\nTry 'install --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a zero-length operand is an error",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}},
			Args:     []string{"a", ""},
			WantErr:  "install: invalid zero-length file name\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunMode(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "an octal mode",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}},
			Args:      []string{"-m", "644", "a", "b"},
			WantFiles: map[string]string{"b": "x"},
			WantMode:  map[string]fs.FileMode{"b": 0o644},
		},
		{
			Name:     "an attached octal mode",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}},
			Args:     []string{"-m644", "a", "b"},
			WantMode: map[string]fs.FileMode{"b": 0o644},
		},
		{
			Name:     "the special bits are set",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}},
			Args:     []string{"-m", "4755", "a", "b"},
			WantMode: map[string]fs.FileMode{"b": fs.ModeSetuid | 0o755},
		},
		{
			Name:     "a symbolic mode starts from no access",
			FS:       fsx.MemFS{"a": {Data: []byte("x"), Mode: 0o666}},
			Args:     []string{"-m", "u=rw,go=r", "a", "b"},
			WantMode: map[string]fs.FileMode{"b": 0o644},
		},
		{
			Name:     "a symbolic mode sets the mode of directories",
			FS:       fsx.MemFS{},
			Args:     []string{"-d", "-m", "u=rwx,go=", "d"},
			WantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o700},
		},
		{
			Name:     "an invalid mode is an error",
			FS:       fsx.MemFS{"a": {}},
			Args:     []string{"-m", "bogus", "a", "b"},
			WantErr:  "install: invalid mode: \"bogus\"\n",
			WantExit: 1,
		},
		{
			Name:     "a missing mode operand is a usage error",
			FS:       fsx.MemFS{"a": {}},
			Args:     []string{"-m"},
			WantErr:  "install: option requires an argument -- 'm'\nTry 'install --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunTimestamps(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "-p preserves the times of the source",
			FS:       fsx.MemFS{"a": {Data: []byte("x"), ModTime: oldTime, Mode: 0o644}},
			Args:     []string{"-p", "a", "b"},
			WantTime: map[string]time.Time{"b": oldTime},
		},
		{
			Name:      "the times are those of the installation without -p",
			FS:        fsx.MemFS{"a": {Data: []byte("x"), ModTime: oldTime, Mode: 0o644}},
			Args:      []string{"a", "b"},
			WantFiles: map[string]string{"b": "x"},
		},
		{
			Name:      "-p applies to an overwritten destination",
			FS:        fsx.MemFS{"a": {Data: []byte("x"), ModTime: oldTime, Mode: 0o644}, "b": {Data: []byte("y"), ModTime: otherTime}},
			Args:      []string{"-p", "a", "b"},
			WantFiles: map[string]string{"b": "x"},
			WantTime:  map[string]time.Time{"b": oldTime},
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunDirectory(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "-d creates one directory",
			FS:       fsx.MemFS{},
			Args:     []string{"-d", "d"},
			WantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o755},
		},
		{
			Name:     "-d creates several directories",
			FS:       fsx.MemFS{},
			Args:     []string{"-d", "a", "b"},
			WantMode: map[string]fs.FileMode{"a": fs.ModeDir | 0o755, "b": fs.ModeDir | 0o755},
		},
		{
			Name: "-d creates the missing parents",
			FS:   fsx.MemFS{"a": {Mode: fs.ModeDir | 0o700}},
			Args: []string{"-d", "a/b/c"},
			WantMode: map[string]fs.FileMode{
				"a":   fs.ModeDir | 0o700,
				"a/b": fs.ModeDir | 0o755,
			},
			Present: []string{"a/b/c"},
		},
		{
			Name:  "-d parents get 755 regardless of the umask",
			FS:    fsx.MemFS{},
			Umask: 0o077,
			Args:  []string{"-d", "a/b"},
			WantMode: map[string]fs.FileMode{
				"a":   fs.ModeDir | 0o755,
				"a/b": fs.ModeDir | 0o755,
			},
		},
		{
			Name: "-d parents get 755 regardless of -m",
			FS:   fsx.MemFS{},
			Args: []string{"-d", "-m", "700", "a/b"},
			WantMode: map[string]fs.FileMode{
				"a":   fs.ModeDir | 0o755,
				"a/b": fs.ModeDir | 0o700,
			},
		},
		{
			Name:     "-d resets the mode of an existing directory",
			FS:       fsx.MemFS{"d": {Mode: fs.ModeDir | 0o700}},
			Args:     []string{"-d", "d"},
			WantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o755},
		},
		{
			Name:     "-d sets the mode of an existing directory with -m",
			FS:       fsx.MemFS{"d": {Mode: fs.ModeDir | 0o755}},
			Args:     []string{"-d", "-m", "700", "d"},
			WantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o700},
		},
		{
			Name:     "-d announces every directory it creates",
			FS:       fsx.MemFS{},
			Args:     []string{"-d", "-v", "a/b"},
			Want:     "install: creating directory 'a'\ninstall: creating directory 'a/b'\n",
			WantMode: map[string]fs.FileMode{"a": fs.ModeDir | 0o755, "a/b": fs.ModeDir | 0o755},
		},
		{
			Name:     "-d keeps quiet about existing directories",
			FS:       fsx.MemFS{"a": {Mode: fs.ModeDir | 0o755}},
			Args:     []string{"-d", "-v", "a/b"},
			Want:     "install: creating directory 'a/b'\n",
			WantMode: map[string]fs.FileMode{"a/b": fs.ModeDir | 0o755},
		},
		{
			Name:     "-d on an existing file is an error",
			FS:       fsx.MemFS{"f": {Data: []byte("x")}},
			Args:     []string{"-d", "f"},
			WantErr:  "install: cannot create directory 'f': file already exists\n",
			WantExit: 1,
		},
		{
			Name:     "-d above an existing file is an error",
			FS:       fsx.MemFS{"f": {Data: []byte("x")}},
			Args:     []string{"-d", "f/d"},
			WantErr:  "install: cannot create directory 'f': not a directory\n",
			WantExit: 1,
		},
		{
			Name:     "-d without an operand is a usage error",
			FS:       fsx.MemFS{},
			Args:     []string{"-d"},
			WantErr:  "install: missing operand\nTry 'install --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunCreateParents(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "-D creates the parents of the destination",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}},
			Args:      []string{"-D", "a", "d/b"},
			WantFiles: map[string]string{"d/b": "x"},
			WantMode:  map[string]fs.FileMode{"d": fs.ModeDir | 0o755, "d/b": 0o755},
		},
		{
			Name:     "-D parents get 755 regardless of the umask",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}},
			Umask:    0o077,
			Args:     []string{"-D", "a", "d/e/b"},
			WantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o755, "d/e": fs.ModeDir | 0o755},
		},
		{
			Name:      "-D announces the directories it creates",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}},
			Args:      []string{"-Dv", "a", "d/b"},
			Want:      "install: creating directory 'd'\n'a' -> 'd/b'\n",
			WantFiles: map[string]string{"d/b": "x"},
		},
		{
			Name:      "-D leaves an existing destination alone",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}, "b": {Data: []byte("y")}},
			Args:      []string{"-D", "a", "b"},
			WantFiles: map[string]string{"b": "x"},
		},
		{
			Name:     "-D above an existing file is an error",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}, "f": {Data: []byte("y")}},
			Args:     []string{"-D", "a", "f/b"},
			WantErr:  "install: cannot create directory 'f': not a directory\n",
			WantExit: 1,
		},
		{
			Name:      "-D -t creates the target directory",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}},
			Args:      []string{"-D", "-t", "d/e", "a"},
			WantFiles: map[string]string{"d/e/a": "x"},
			WantMode:  map[string]fs.FileMode{"d": fs.ModeDir | 0o755, "d/e": fs.ModeDir | 0o755},
		},
		{
			Name:     "-D -t refuses a file target",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}, "d": {Mode: fs.ModeDir | 0o755}, "d/f": {Data: []byte("y")}},
			Args:     []string{"-D", "-t", "d/f", "a"},
			WantErr:  "install: failed to access 'd/f': not a directory\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunBackup(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "-b makes a simple backup by default",
			FS:        fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			Args:      []string{"-b", "a", "b"},
			WantFiles: map[string]string{"b": "new", "b~": "old"},
		},
		{
			Name:      "-S overrides the suffix",
			FS:        fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			Args:      []string{"-b", "-S", ".bak", "a", "b"},
			WantFiles: map[string]string{"b": "new", "b.bak": "old"},
		},
		{
			Name:      "SIMPLE_BACKUP_SUFFIX sets the suffix",
			FS:        fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			Environ:   []string{"SIMPLE_BACKUP_SUFFIX=.old"},
			Args:      []string{"-b", "a", "b"},
			WantFiles: map[string]string{"b": "new", "b.old": "old"},
		},
		{
			Name:      "--backup=numbered numbers the backups",
			FS:        fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			Args:      []string{"--backup=numbered", "a", "b"},
			WantFiles: map[string]string{"b": "new", "b.~1~": "old"},
		},
		{
			Name:      "numbered backups continue after the highest",
			FS:        fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}, "b.~2~": {Data: []byte("older")}},
			Args:      []string{"--backup=numbered", "a", "b"},
			WantFiles: map[string]string{"b": "new", "b.~3~": "old"},
		},
		{
			Name:      "--backup=existing follows numbered ones",
			FS:        fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}, "b.~1~": {Data: []byte("older")}},
			Args:      []string{"--backup=existing", "a", "b"},
			WantFiles: map[string]string{"b": "new", "b.~2~": "old"},
		},
		{
			Name:      "VERSION_CONTROL sets the method",
			FS:        fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			Environ:   []string{"VERSION_CONTROL=numbered"},
			Args:      []string{"-b", "a", "b"},
			WantFiles: map[string]string{"b": "new", "b.~1~": "old"},
		},
		{
			Name:      "an invalid VERSION_CONTROL is ignored with a warning",
			FS:        fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			Environ:   []string{"VERSION_CONTROL=bogus"},
			Args:      []string{"-b", "a", "b"},
			WantFiles: map[string]string{"b": "new", "b~": "old"},
			WantErr:   "install: ignoring invalid value of environment variable VERSION_CONTROL\n",
		},
		{
			Name:      "a set but empty SIMPLE_BACKUP_SUFFIX is honored",
			FS:        fsx.MemFS{"a": {Data: []byte("n")}, "b": {Data: []byte("old")}},
			Environ:   []string{"SIMPLE_BACKUP_SUFFIX="},
			Args:      []string{"-b", "a", "b"},
			WantFiles: map[string]string{"b": "n"},
		},
		{
			Name:     "an invalid backup method is a usage error",
			FS:       fsx.MemFS{"a": {}, "b": {}},
			Args:     []string{"--backup=bogus", "a", "b"},
			WantErr:  "install: invalid backup method 'bogus'\nTry 'install --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:      "-b backups a directory destination with -T",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}, "d": {Mode: fs.ModeDir | 0o700}},
			Args:      []string{"-bT", "a", "d"},
			WantFiles: map[string]string{"d": "x"},
			WantMode:  map[string]fs.FileMode{"d~": fs.ModeDir | 0o700},
		},
		{
			Name:      "verbose tells about the backup",
			FS:        fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			Args:      []string{"-bv", "a", "b"},
			Want:      "'a' -> 'b' (backup: 'b~')\n",
			WantFiles: map[string]string{"b": "new", "b~": "old"},
		},
		{
			Name:     "a backup which would destroy the source is refused",
			FS:       fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}, "b~": {Data: []byte("new")}},
			Args:     []string{"-b", "b~", "b"},
			WantErr:  "install: backing up 'b' might destroy source; 'b~' not copied\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunCompare(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "-C skips an identical destination",
			FS:        fsx.MemFS{"a": {Data: []byte("x"), Mode: 0o755}, "b": {Data: []byte("x"), Mode: 0o755, ModTime: otherTime}},
			Args:      []string{"-C", "a", "b"},
			WantFiles: map[string]string{"b": "x"},
			WantTime:  map[string]time.Time{"b": otherTime},
		},
		{
			Name:      "-C installs a different content",
			FS:        fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old"), Mode: 0o600}},
			Args:      []string{"-C", "a", "b"},
			WantFiles: map[string]string{"b": "new"},
			WantMode:  map[string]fs.FileMode{"b": 0o755},
		},
		{
			Name:     "-C installs a different mode",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}, "b": {Data: []byte("x"), Mode: 0o600}},
			Args:     []string{"-C", "a", "b"},
			WantMode: map[string]fs.FileMode{"b": 0o755},
		},
		{
			Name:      "-C installs a longer content",
			FS:        fsx.MemFS{"a": {Data: []byte("longer")}, "b": {Data: []byte("x"), Mode: 0o755}},
			Args:      []string{"-C", "a", "b"},
			WantFiles: map[string]string{"b": "longer"},
		},
		{
			Name:      "-C installs a shorter content",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}, "b": {Data: []byte("longer"), Mode: 0o755}},
			Args:      []string{"-C", "a", "b"},
			WantFiles: map[string]string{"b": "x"},
		},
		{
			Name:      "-C installs a missing destination",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}},
			Args:      []string{"-C", "a", "b"},
			WantFiles: map[string]string{"b": "x"},
			WantMode:  map[string]fs.FileMode{"b": 0o755},
		},
		{
			Name:      "-C replaces a destination which is a symbolic link",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}, "c": {Data: []byte("y")}, "l": link("c")},
			Args:      []string{"-C", "a", "l"},
			WantFiles: map[string]string{"l": "x", "c": "y"},
			WantMode:  map[string]fs.FileMode{"l": 0o755},
		},
		{
			Name:     "-C with -p still sets the timestamps",
			FS:       fsx.MemFS{"a": {Data: []byte("x"), ModTime: oldTime, Mode: 0o755}, "b": {Data: []byte("x"), Mode: 0o755, ModTime: otherTime}},
			Args:     []string{"-Cp", "a", "b"},
			WantTime: map[string]time.Time{"b": oldTime},
		},
		{
			Name:      "-C with a special mode warns and always installs",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}, "b": {Data: []byte("x"), Mode: fs.ModeSetuid | 0o755}},
			Args:      []string{"-C", "-m", "4755", "a", "b"},
			WantFiles: map[string]string{"b": "x"},
			WantMode:  map[string]fs.FileMode{"b": fs.ModeSetuid | 0o755},
			WantErr:   "install: the --compare (-C) option is ignored when you specify a mode with non-permission bits\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOwnership(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ownership is not set on windows")
	}
	tests := []testutil.Case{
		{
			Name:      "-o and -g set the ownership",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}},
			Args:      []string{"-o", "100", "-g", "200", "a", "b"},
			WantFiles: map[string]string{"b": "x"},
			WantMode:  map[string]fs.FileMode{"b": 0o755},
		},
		{
			Name:     "-g sets the group of a directory",
			FS:       fsx.MemFS{},
			Args:     []string{"-d", "-g", "200", "d"},
			WantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o755},
		},
		{
			Name:     "an invalid owner is an error",
			FS:       fsx.MemFS{"a": {}},
			Args:     []string{"-o", "bogus", "a", "b"},
			WantErr:  "install: invalid user 'bogus'\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid group is an error",
			FS:       fsx.MemFS{"a": {}},
			Args:     []string{"-g", "bogus", "a", "b"},
			WantErr:  "install: invalid group 'bogus'\n",
			WantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testutil.Once(t, New, test)
			if test.WantExit != 0 {
				return
			}
			dst := strings.TrimSuffix(test.Args[len(test.Args)-1], "/")
			var wantUid, wantGid int
			for i, arg := range test.Args {
				switch arg {
				case "-o":
					wantUid, _ = parseOwner(test.Args[i+1])
				case "-g":
					wantGid, _ = parseGroup(test.Args[i+1])
				}
			}
			fi, err := fs.Stat(test.FS, dst)
			if err != nil {
				t.Fatalf("Stat(%q) error = %v", dst, err)
			}
			uid, gid, ok := fsx.Owner(fi)
			if !ok {
				t.Fatalf("Owner(%q) not available", dst)
			}
			if uid != wantUid || gid != wantGid {
				t.Errorf("%s owner = %d:%d, want %d:%d", dst, uid, gid, wantUid, wantGid)
			}
		})
	}
}

func TestRunSymlinks(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "a symbolic link source is followed",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}, "l": link("a")},
			Args:      []string{"l", "b"},
			WantFiles: map[string]string{"b": "x"},
			WantMode:  map[string]fs.FileMode{"b": 0o755},
		},
		{
			Name:     "a dangling symbolic link source is an error",
			FS:       fsx.MemFS{"l": link("missing")},
			Args:     []string{"l", "b"},
			WantErr:  "install: cannot stat 'l': file does not exist\n",
			WantExit: 1,
		},
		{
			Name:      "a symbolic link destination is replaced",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}, "b": {Data: []byte("y")}, "l": link("b")},
			Args:      []string{"a", "l"},
			WantFiles: map[string]string{"l": "x", "b": "y"},
			WantMode:  map[string]fs.FileMode{"l": 0o755},
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunVerbose(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "verbose tells about each file",
			FS:        fsx.MemFS{"a": {Data: []byte("1")}, "b": {Data: []byte("2")}, "d": {Mode: fs.ModeDir | 0o755}},
			Args:      []string{"-v", "a", "b", "d"},
			Want:      "'a' -> 'd/a'\n'b' -> 'd/b'\n",
			WantFiles: map[string]string{"d/a": "1", "d/b": "2"},
		},
		{
			Name:      "verbose tells about a replaced destination",
			FS:        fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			Args:      []string{"-v", "a", "b"},
			Want:      "removed 'b'\n'a' -> 'b'\n",
			WantFiles: map[string]string{"b": "new"},
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "--directory",
			FS:       fsx.MemFS{},
			Args:     []string{"--directory", "d"},
			WantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o755},
		},
		{
			Name:     "--mode with an attached value",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}},
			Args:     []string{"--mode=600", "a", "b"},
			WantMode: map[string]fs.FileMode{"b": 0o600},
		},
		{
			Name:     "--mode with the next argument",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}},
			Args:     []string{"--mode", "600", "a", "b"},
			WantMode: map[string]fs.FileMode{"b": 0o600},
		},
		{
			Name:      "--target-directory",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}, "d": {Mode: fs.ModeDir | 0o755}},
			Args:      []string{"--target-directory=d", "a"},
			WantFiles: map[string]string{"d/a": "x"},
		},
		{
			Name:      "--no-target-directory",
			FS:        fsx.MemFS{"a": {Data: []byte("x")}, "d": {Mode: fs.ModeDir | 0o755}},
			Args:      []string{"--no-target-directory", "a", "f"},
			WantFiles: map[string]string{"f": "x"},
		},
		{
			Name:     "--compare",
			FS:       fsx.MemFS{"a": {Data: []byte("x")}, "b": {Data: []byte("x"), Mode: 0o755}},
			Args:     []string{"--compare", "a", "b"},
			WantMode: map[string]fs.FileMode{"b": 0o755},
		},
		{
			Name:     "--preserve-timestamps",
			FS:       fsx.MemFS{"a": {Data: []byte("x"), ModTime: oldTime, Mode: 0o644}},
			Args:     []string{"--preserve-timestamps", "a", "b"},
			WantTime: map[string]time.Time{"b": oldTime},
		},
		{
			Name:      "--suffix",
			FS:        fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			Args:      []string{"--backup", "--suffix=.bak", "a", "b"},
			WantFiles: map[string]string{"b": "new", "b.bak": "old"},
		},
		{
			Name:     "an unknown long option is a usage error",
			FS:       fsx.MemFS{},
			Args:     []string{"--bogus"},
			WantErr:  "install: unrecognized option '--bogus'\nTry 'install --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown short option is a usage error",
			FS:       fsx.MemFS{},
			Args:     []string{"-x"},
			WantErr:  "install: invalid option -- 'x'\nTry 'install --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an argument to a flag is a usage error",
			FS:       fsx.MemFS{},
			Args:     []string{"--verbose=yes"},
			WantErr:  "install: option '--verbose=yes' doesn't allow an argument\nTry 'install --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunIgnoredOption(t *testing.T) {
	testutil.Once(t, New, testutil.Case{
		Name:      "-c is ignored",
		FS:        fsx.MemFS{"a": {Data: []byte("x")}},
		Args:      []string{"-c", "a", "b"},
		WantFiles: map[string]string{"b": "x"},
		WantMode:  map[string]fs.FileMode{"b": 0o755},
	})
}

func TestRunOptionsAfterOperands(t *testing.T) {
	testutil.Once(t, New, testutil.Case{
		Name:      "an option after an operand is an operand",
		FS:        fsx.MemFS{"a": {Data: []byte("x")}, "-v": {Data: []byte("y")}, "d": {Mode: fs.ModeDir | 0o755}},
		Args:      []string{"a", "-v", "d"},
		WantFiles: map[string]string{"d/a": "x", "d/-v": "y"},
		Want:      "",
	})
}

func TestRunContext(t *testing.T) {
	testutil.Cancelled(t, New, nil, fsx.MemFS{"a": {Data: []byte("x")}}, "a", "b")
}

func TestCompareContext(t *testing.T) {
	tests := []struct {
		Name  string
		Files fsx.MemFS
	}{
		{
			Name:  "empty files",
			Files: fsx.MemFS{"a": {Mode: 0o755}, "b": {Mode: 0o755}},
		},
		{
			Name:  "files with content",
			Files: fsx.MemFS{"a": {Data: []byte("x"), Mode: 0o755}, "b": {Data: []byte("x"), Mode: 0o755}},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			p := &Prog{Runtime: &coreutils.Runtime{FS: test.Files}}
			fi, err := fs.Stat(test.Files, "a")
			if err != nil {
				t.Fatal(err)
			}
			dfi, err := fs.Stat(test.Files, "b")
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.upToDate(ctx, fsop.Pair{Src: "a", Dst: "b"}, fi, dfi, settings{fileMode: 0o755})
			if !errors.Is(err, context.Canceled) {
				t.Errorf("upToDate() error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestCompareContextFinalRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	files := cancelFS{FS: fsx.MemFS{"a": {Data: []byte("x"), Mode: 0o755}, "b": {Data: []byte("x"), Mode: 0o755}}, cancel: cancel}

	p := &Prog{Runtime: &coreutils.Runtime{FS: files}}
	fi, err := fs.Stat(files.FS, "a")
	if err != nil {
		t.Fatal(err)
	}
	dfi, err := fs.Stat(files.FS, "b")
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.upToDate(ctx, fsop.Pair{Src: "a", Dst: "b"}, fi, dfi, settings{fileMode: 0o755})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("upToDate() error = %v, want context.Canceled", err)
	}
}

// cancelFS cancels its context when the second read of the destination file
// happens, the one which checks for extra bytes after the compared ones.
type cancelFS struct {
	fsx.FS
	cancel context.CancelFunc
}

func (f cancelFS) Open(name string) (fs.File, error) {
	file, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	if name != "b" {
		return file, nil
	}
	return &cancelFile{cancel: f.cancel, File: file}, nil
}

// cancelFile cancels the context of the enclosing test on its second read.
type cancelFile struct {
	fs.File
	cancel context.CancelFunc
	reads  int
}

func (f *cancelFile) Read(p []byte) (int, error) {
	f.reads++
	if f.reads == 2 {
		f.cancel()
	}
	return f.File.Read(p)
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "install", usage)
}
