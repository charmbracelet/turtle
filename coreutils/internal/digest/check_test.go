package digest

import (
	"runtime"
	"strings"
	"testing"

	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// checksumLine builds a checksum line as the utility writes it, with the
// character which tells the text mode.
func checksumLine(digest, name string) string {
	return digest + "  " + name
}

// taggedLine builds a checksum line as the utility writes it with --tag.
func taggedLine(tag, digest, name string) string {
	return tag + " (" + name + ") = " + digest
}

// testFiles returns the fixtures of testFS with the given extra files.
func testFiles(extra map[string]string) fsx.MemoryFS {
	files := testFS()
	for name, data := range extra {
		files[name] = &fsx.MemoryFile{Data: []byte(data)}
	}
	return files
}

// sumsFS returns the fixtures with a checksum file named "sums.txt" holding the
// given lines.
func sumsFS(lines ...string) fsx.MemoryFS {
	return testFiles(map[string]string{
		"sums.txt": strings.Join(lines, "\n") + "\n",
	})
}

func TestRunCheck(t *testing.T) {
	good := checksumLine(helloSHA256, "hello.txt")

	tests := []testutil.Case{
		{
			Name: "a matching checksum",
			FS:   sumsFS(good),
			Args: []string{"-c", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name: "the long check option",
			FS:   sumsFS(good),
			Args: []string{"--check", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name:     "a checksum which doesn't match",
			FS:       sumsFS(checksumLine(fooSHA256, "hello.txt")),
			Args:     []string{"-c", "sums.txt"},
			Want:     "hello.txt: FAILED\n",
			WantErr:  "sha256sum: WARNING: 1 computed checksum did NOT match\n",
			WantExit: 1,
		},
		{
			Name: "several mismatches are told in the plural",
			FS: sumsFS(
				checksumLine(fooSHA256, "hello.txt"),
				checksumLine(helloSHA256, "foo.txt"),
			),
			Args:     []string{"-c", "sums.txt"},
			Want:     "hello.txt: FAILED\nfoo.txt: FAILED\n",
			WantErr:  "sha256sum: WARNING: 2 computed checksums did NOT match\n",
			WantExit: 1,
		},
		{
			Name: "a file which isn't there",
			FS:   sumsFS(checksumLine(helloSHA256, "missing.txt")),
			Args: []string{"-c", "sums.txt"},
			Want: "missing.txt: FAILED open or read\n",
			WantErr: "sha256sum: missing.txt: file does not exist\n" +
				"sha256sum: WARNING: 1 listed file could not be read\n",
			WantExit: 1,
		},
		{
			Name: "unreadable files are told in the plural",
			FS: sumsFS(
				checksumLine(helloSHA256, "missing.txt"),
				checksumLine(helloSHA256, "other.txt"),
			),
			Args: []string{"-c", "sums.txt"},
			Want: "missing.txt: FAILED open or read\nother.txt: FAILED open or read\n",
			WantErr: "sha256sum: missing.txt: file does not exist\n" +
				"sha256sum: other.txt: file does not exist\n" +
				"sha256sum: WARNING: 2 listed files could not be read\n",
			WantExit: 1,
		},
		{
			Name:     "a file with no checksum lines",
			FS:       sumsFS("not a checksum"),
			Args:     []string{"-c", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name:     "an empty checksum file",
			FS:       testFiles(map[string]string{"sums.txt": ""}),
			Args:     []string{"-c", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name:     "a file holding only blank lines and comments",
			FS:       sumsFS("# written by sha256sum", ""),
			Args:     []string{"-c", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name:    "improperly formatted lines don't fail the run",
			FS:      sumsFS(good, "junk"),
			Args:    []string{"-c", "sums.txt"},
			Want:    "hello.txt: OK\n",
			WantErr: "sha256sum: WARNING: 1 line is improperly formatted\n",
		},
		{
			Name: "warn tells which line is bad",
			FS:   sumsFS(good, "junk"),
			Args: []string{"-c", "-w", "sums.txt"},
			Want: "hello.txt: OK\n",
			WantErr: "sha256sum: sums.txt: 2: improperly formatted SHA256 checksum line\n" +
				"sha256sum: WARNING: 1 line is improperly formatted\n",
		},
		{
			Name:     "strict fails on a bad line",
			FS:       sumsFS(good, "junk"),
			Args:     []string{"-c", "--strict", "sums.txt"},
			Want:     "hello.txt: OK\n",
			WantErr:  "sha256sum: WARNING: 1 line is improperly formatted\n",
			WantExit: 1,
		},
		{
			Name:     "status holds everything back",
			FS:       sumsFS(checksumLine(fooSHA256, "hello.txt")),
			Args:     []string{"-c", "--status", "sums.txt"},
			WantExit: 1,
		},
		{
			Name: "status with a matching checksum",
			FS:   sumsFS(good),
			Args: []string{"-c", "--status", "sums.txt"},
		},
		{
			Name:     "status still reports a file which can't be read",
			FS:       sumsFS(checksumLine(helloSHA256, "missing.txt")),
			Args:     []string{"-c", "--status", "sums.txt"},
			WantErr:  "sha256sum: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "status still reports a file with no checksum lines",
			FS:       sumsFS("junk"),
			Args:     []string{"-c", "--status", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name:     "quiet hides the OK lines",
			FS:       sumsFS(good, checksumLine(helloSHA256, "foo.txt")),
			Args:     []string{"-c", "--quiet", "sums.txt"},
			Want:     "foo.txt: FAILED\n",
			WantErr:  "sha256sum: WARNING: 1 computed checksum did NOT match\n",
			WantExit: 1,
		},
		{
			Name:     "status given after quiet holds everything back",
			FS:       sumsFS(good, "junk"),
			Args:     []string{"-c", "--quiet", "--status", "sums.txt"},
			WantExit: 0,
		},
		{
			Name:    "quiet given after status holds the OK lines back",
			FS:      sumsFS(good, "junk"),
			Args:    []string{"-c", "--status", "--quiet", "sums.txt"},
			WantErr: "sha256sum: WARNING: 1 line is improperly formatted\n",
		},
		{
			Name: "warn given after status speaks again",
			FS:   sumsFS(good, "junk"),
			Args: []string{"-c", "--status", "-w", "sums.txt"},
			Want: "hello.txt: OK\n",
			WantErr: "sha256sum: sums.txt: 2: improperly formatted SHA256 checksum line\n" +
				"sha256sum: WARNING: 1 line is improperly formatted\n",
		},
		{
			Name: "ignore-missing skips a file which isn't there",
			FS:   sumsFS(checksumLine(helloSHA256, "missing.txt"), good),
			Args: []string{"-c", "--ignore-missing", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name:     "ignore-missing fails when nothing was verified",
			FS:       sumsFS(checksumLine(helloSHA256, "missing.txt")),
			Args:     []string{"-c", "--ignore-missing", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no file was verified\n",
			WantExit: 1,
		},
		{
			Name: "ignore-missing doesn't hide a mismatch",
			FS:   sumsFS(checksumLine(fooSHA256, "hello.txt")),
			Args: []string{"-c", "--ignore-missing", "sums.txt"},
			Want: "hello.txt: FAILED\n",
			WantErr: "sha256sum: WARNING: 1 computed checksum did NOT match\n" +
				"sha256sum: sums.txt: no file was verified\n",
			WantExit: 1,
		},
		{
			Name:     "a checksum file which isn't there",
			FS:       testFS(),
			Args:     []string{"-c", "missing.txt"},
			WantErr:  "sha256sum: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "several checksum files are all read",
			FS: testFiles(map[string]string{
				"sums.txt":  good + "\n",
				"sums2.txt": good + "\n",
			}),
			Args: []string{"-c", "sums.txt", "sums2.txt"},
			Want: "hello.txt: OK\nhello.txt: OK\n",
		},
		{
			Name: "a checksum file with no good lines fails the run",
			FS: testFiles(map[string]string{
				"sums.txt":  good + "\n",
				"sums2.txt": "junk\n",
			}),
			Args:     []string{"-c", "sums.txt", "sums2.txt"},
			Want:     "hello.txt: OK\n",
			WantErr:  "sha256sum: sums2.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name: "the last line may come without a newline",
			FS:   testFiles(map[string]string{"sums.txt": good}),
			Args: []string{"-c", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
	}

	testutil.Run(t, newSHA256, tests)
}

func TestRunCheckLineFormats(t *testing.T) {
	good := checksumLine(helloSHA256, "hello.txt")

	tests := []testutil.Case{
		{
			Name: "a line of the standard input format",
			FS:   sumsFS(helloSHA256 + " hello.txt"),
			Args: []string{"-c", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name: "a binary mode line",
			FS:   sumsFS(helloSHA256 + " *hello.txt"),
			Args: []string{"-c", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name: "a BSD style line",
			FS:   sumsFS(taggedLine("SHA256", helloSHA256, "hello.txt")),
			Args: []string{"-c", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name: "a BSD style line written without blanks",
			FS:   sumsFS("SHA256(hello.txt)=" + helloSHA256),
			Args: []string{"-c", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name:     "a BSD style line of another algorithm",
			FS:       sumsFS(taggedLine("MD5", helloMD5, "hello.txt")),
			Args:     []string{"-c", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name:     "a BSD style line without a digest",
			FS:       sumsFS("SHA256 (hello.txt) ="),
			Args:     []string{"-c", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name: "a BSD style line naming a file with a parenthesis",
			FS:   sumsFS(taggedLine("SHA256", helloSHA256, "a)b.txt")),
			Args: []string{"-c", "sums.txt"},
			Want: "a)b.txt: OK\n",
		},
		{
			Name:     "a BSD style line without a closing parenthesis",
			FS:       sumsFS("SHA256 (hello.txt"),
			Args:     []string{"-c", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name:     "a BSD style line without the equals sign",
			FS:       sumsFS("SHA256 (hello.txt) " + helloSHA256),
			Args:     []string{"-c", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name: "an uppercase digest",
			FS:   sumsFS(strings.ToUpper(helloSHA256) + "  hello.txt"),
			Args: []string{"-c", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name: "leading blanks",
			FS:   sumsFS("\t  " + good),
			Args: []string{"-c", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name: "a file name holding a leading blank",
			FS:   sumsFS(helloSHA256 + "   lead.txt"),
			Args: []string{"-c", "sums.txt"},
			Want: " lead.txt: OK\n",
		},
		{
			Name: "CRLF line endings",
			FS:   testFiles(map[string]string{"sums.txt": good + "\r\n"}),
			Args: []string{"-c", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name: "an escaped file name",
			FS:   sumsFS(`\` + helloSHA256 + `  we\nird.txt`),
			Args: []string{"-c", "sums.txt"},
			Want: "we\nird.txt: OK\n",
		},
		{
			Name: "an escaped carriage return",
			FS:   sumsFS(`\` + helloSHA256 + `  cr\rname.txt`),
			Args: []string{"-c", "sums.txt"},
			Want: "cr\rname.txt: OK\n",
		},
		{
			Name: "a BSD style line with an escaped file name",
			FS:   sumsFS(`\SHA256 (we\nird.txt) = ` + helloSHA256),
			Args: []string{"-c", "sums.txt"},
			Want: "we\nird.txt: OK\n",
		},
		{
			Name:     "an unknown escape sequence",
			FS:       sumsFS(`\` + helloSHA256 + `  we\qird.txt`),
			Args:     []string{"-c", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name:     "a file name ending with a backslash",
			FS:       sumsFS(`\` + helloSHA256 + `  broken\`),
			Args:     []string{"-c", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name:     "a digest in the wrong length",
			FS:       sumsFS(helloSHA256[:60] + "  hello.txt"),
			Args:     []string{"-c", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name:     "a line which is too short",
			FS:       sumsFS("sha256sum"),
			Args:     []string{"-c", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name:     "a digest which isn't hexadecimal",
			FS:       sumsFS(strings.Repeat("z", len(helloSHA256)) + "  hello.txt"),
			Args:     []string{"-c", "sums.txt"},
			WantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name: "a line with a one character file name",
			FS:   sumsFS(helloSHA256 + " x"),
			Args: []string{"-c", "sums.txt"},
			Want: "x: FAILED open or read\n",
			WantErr: "sha256sum: x: file does not exist\n" +
				"sha256sum: WARNING: 1 listed file could not be read\n",
			WantExit: 1,
		},
		{
			Name:    "the formats of the mode character can't be mixed",
			FS:      sumsFS(good, helloSHA256+" empty.txt"),
			Args:    []string{"-c", "sums.txt"},
			Want:    "hello.txt: OK\n",
			WantErr: "sha256sum: WARNING: 1 line is improperly formatted\n",
		},
	}

	testutil.Run(t, newSHA256, tests)
}

// TestRunCheckBackslash covers the escaped names which a Windows file system
// can't hold.
func TestRunCheckBackslash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("not supported on %s", "windows")
	}
	testutil.Once(t, newSHA256, testutil.Case{
		FS:   sumsFS(`\` + helloSHA256 + `  back\\slash.txt`),
		Args: []string{"-c", "sums.txt"},
		Want: `back\slash.txt: OK` + "\n",
	})
}

func TestRunCheckStandardInput(t *testing.T) {
	good := checksumLine(helloSHA256, "hello.txt")

	tests := []testutil.Case{
		{
			Name:     "a checksum file which can't be read",
			FS:       testFS(),
			Stdin:    testutil.ErrorReader{},
			Args:     []string{"-c"},
			WantErr:  "sha256sum: standard input: read error: boom\n",
			WantExit: 1,
		},
		{
			Name:  "no operands read the checksum lines from the standard input",
			FS:    testFS(),
			Stdin: strings.NewReader(good + "\n"),
			Args:  []string{"-c"},
			Want:  "hello.txt: OK\n",
		},
		{
			Name:  "dash reads the checksum lines from the standard input",
			FS:    testFS(),
			Stdin: strings.NewReader(good + "\n"),
			Args:  []string{"-c", "-"},
			Want:  "hello.txt: OK\n",
		},
		{
			Name: "the standard input is named in the diagnostics",
			FS:   testFS(),
			Stdin: strings.NewReader(helloSHA256 + "  missing.txt\n" +
				checksumLine(helloSHA256, "-") + "\n"),
			Args: []string{"-c"},
			Want: "missing.txt: FAILED open or read\n",
			WantErr: "sha256sum: missing.txt: file does not exist\n" +
				"sha256sum: WARNING: 1 line is improperly formatted\n" +
				"sha256sum: WARNING: 1 listed file could not be read\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, newSHA256, tests)
}

func TestRunCheckPerAlgorithm(t *testing.T) {
	for _, test := range []struct {
		algorithm Algorithm
		digest    string
	}{
		{testMD5, helloMD5},
		{testSHA1, helloSHA1},
		{testSHA224, helloSHA224},
		{testSHA256, helloSHA256},
		{testSHA384, helloSHA384},
		{testSHA512, helloSHA512},
	} {
		t.Run(test.algorithm.Name, func(t *testing.T) {
			testutil.Once(t, newAlgorithm(test.algorithm), testutil.Case{
				FS:   sumsFS(checksumLine(test.digest, "hello.txt")),
				Args: []string{"-c", "sums.txt"},
				Want: "hello.txt: OK\n",
			})
			// A line holding a digest which no algorithm of the family has
			// must be rejected.
			testutil.Once(t, newAlgorithm(test.algorithm), testutil.Case{
				FS:   sumsFS(checksumLine(strings.Repeat("ab", 10), "hello.txt")),
				Args: []string{"-c", "sums.txt"},
				WantErr: test.algorithm.Name +
					": sums.txt: no properly formatted checksum lines found\n",
				WantExit: 1,
			})
		})
	}
}
