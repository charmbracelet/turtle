package digest

import (
	"strings"
	"testing"

	"charm.land/snail/fsx"
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
func testFiles(extra map[string]string) fsx.MemFS {
	files := testFS()
	for name, data := range extra {
		files[name] = &fsx.MemFile{Data: []byte(data)}
	}
	return files
}

// sumsFS returns the fixtures with a checksum file named "sums.txt" holding the
// given lines.
func sumsFS(lines ...string) fsx.MemFS {
	return testFiles(map[string]string{
		"sums.txt": strings.Join(lines, "\n") + "\n",
	})
}

func TestRunCheck(t *testing.T) {
	good := checksumLine(helloSHA256, "hello.txt")

	tests := []testCase{
		{
			name: "a matching checksum",
			fsys: sumsFS(good),
			args: []string{"-c", "sums.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name: "the long check option",
			fsys: sumsFS(good),
			args: []string{"--check", "sums.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name:     "a checksum which doesn't match",
			fsys:     sumsFS(checksumLine(fooSHA256, "hello.txt")),
			args:     []string{"-c", "sums.txt"},
			want:     "hello.txt: FAILED\n",
			wantErr:  "sha256sum: WARNING: 1 computed checksum did NOT match\n",
			wantExit: 1,
		},
		{
			name: "several mismatches are told in the plural",
			fsys: sumsFS(
				checksumLine(fooSHA256, "hello.txt"),
				checksumLine(helloSHA256, "foo.txt"),
			),
			args:     []string{"-c", "sums.txt"},
			want:     "hello.txt: FAILED\nfoo.txt: FAILED\n",
			wantErr:  "sha256sum: WARNING: 2 computed checksums did NOT match\n",
			wantExit: 1,
		},
		{
			name: "a file which isn't there",
			fsys: sumsFS(checksumLine(helloSHA256, "missing.txt")),
			args: []string{"-c", "sums.txt"},
			want: "missing.txt: FAILED open or read\n",
			wantErr: "sha256sum: missing.txt: file does not exist\n" +
				"sha256sum: WARNING: 1 listed file could not be read\n",
			wantExit: 1,
		},
		{
			name: "unreadable files are told in the plural",
			fsys: sumsFS(
				checksumLine(helloSHA256, "missing.txt"),
				checksumLine(helloSHA256, "other.txt"),
			),
			args: []string{"-c", "sums.txt"},
			want: "missing.txt: FAILED open or read\nother.txt: FAILED open or read\n",
			wantErr: "sha256sum: missing.txt: file does not exist\n" +
				"sha256sum: other.txt: file does not exist\n" +
				"sha256sum: WARNING: 2 listed files could not be read\n",
			wantExit: 1,
		},
		{
			name:     "a file with no checksum lines",
			fsys:     sumsFS("not a checksum"),
			args:     []string{"-c", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name:     "an empty checksum file",
			fsys:     testFiles(map[string]string{"sums.txt": ""}),
			args:     []string{"-c", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name:     "a file holding only blank lines and comments",
			fsys:     sumsFS("# written by sha256sum", ""),
			args:     []string{"-c", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name:    "improperly formatted lines don't fail the run",
			fsys:    sumsFS(good, "junk"),
			args:    []string{"-c", "sums.txt"},
			want:    "hello.txt: OK\n",
			wantErr: "sha256sum: WARNING: 1 line is improperly formatted\n",
		},
		{
			name: "warn tells which line is bad",
			fsys: sumsFS(good, "junk"),
			args: []string{"-c", "-w", "sums.txt"},
			want: "hello.txt: OK\n",
			wantErr: "sha256sum: sums.txt: 2: improperly formatted SHA256 checksum line\n" +
				"sha256sum: WARNING: 1 line is improperly formatted\n",
		},
		{
			name:     "strict fails on a bad line",
			fsys:     sumsFS(good, "junk"),
			args:     []string{"-c", "--strict", "sums.txt"},
			want:     "hello.txt: OK\n",
			wantErr:  "sha256sum: WARNING: 1 line is improperly formatted\n",
			wantExit: 1,
		},
		{
			name:     "status holds everything back",
			fsys:     sumsFS(checksumLine(fooSHA256, "hello.txt")),
			args:     []string{"-c", "--status", "sums.txt"},
			wantExit: 1,
		},
		{
			name: "status with a matching checksum",
			fsys: sumsFS(good),
			args: []string{"-c", "--status", "sums.txt"},
		},
		{
			name:     "status still reports a file which can't be read",
			fsys:     sumsFS(checksumLine(helloSHA256, "missing.txt")),
			args:     []string{"-c", "--status", "sums.txt"},
			wantErr:  "sha256sum: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "status still reports a file with no checksum lines",
			fsys:     sumsFS("junk"),
			args:     []string{"-c", "--status", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name:     "quiet hides the OK lines",
			fsys:     sumsFS(good, checksumLine(helloSHA256, "foo.txt")),
			args:     []string{"-c", "--quiet", "sums.txt"},
			want:     "foo.txt: FAILED\n",
			wantErr:  "sha256sum: WARNING: 1 computed checksum did NOT match\n",
			wantExit: 1,
		},
		{
			name:     "status given after quiet holds everything back",
			fsys:     sumsFS(good, "junk"),
			args:     []string{"-c", "--quiet", "--status", "sums.txt"},
			wantExit: 0,
		},
		{
			name:    "quiet given after status holds the OK lines back",
			fsys:    sumsFS(good, "junk"),
			args:    []string{"-c", "--status", "--quiet", "sums.txt"},
			wantErr: "sha256sum: WARNING: 1 line is improperly formatted\n",
		},
		{
			name: "warn given after status speaks again",
			fsys: sumsFS(good, "junk"),
			args: []string{"-c", "--status", "-w", "sums.txt"},
			want: "hello.txt: OK\n",
			wantErr: "sha256sum: sums.txt: 2: improperly formatted SHA256 checksum line\n" +
				"sha256sum: WARNING: 1 line is improperly formatted\n",
		},
		{
			name: "ignore-missing skips a file which isn't there",
			fsys: sumsFS(checksumLine(helloSHA256, "missing.txt"), good),
			args: []string{"-c", "--ignore-missing", "sums.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name:     "ignore-missing fails when nothing was verified",
			fsys:     sumsFS(checksumLine(helloSHA256, "missing.txt")),
			args:     []string{"-c", "--ignore-missing", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no file was verified\n",
			wantExit: 1,
		},
		{
			name: "ignore-missing doesn't hide a mismatch",
			fsys: sumsFS(checksumLine(fooSHA256, "hello.txt")),
			args: []string{"-c", "--ignore-missing", "sums.txt"},
			want: "hello.txt: FAILED\n",
			wantErr: "sha256sum: WARNING: 1 computed checksum did NOT match\n" +
				"sha256sum: sums.txt: no file was verified\n",
			wantExit: 1,
		},
		{
			name:     "a checksum file which isn't there",
			fsys:     testFS(),
			args:     []string{"-c", "missing.txt"},
			wantErr:  "sha256sum: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name: "several checksum files are all read",
			fsys: testFiles(map[string]string{
				"sums.txt":  good + "\n",
				"sums2.txt": good + "\n",
			}),
			args: []string{"-c", "sums.txt", "sums2.txt"},
			want: "hello.txt: OK\nhello.txt: OK\n",
		},
		{
			name: "a checksum file with no good lines fails the run",
			fsys: testFiles(map[string]string{
				"sums.txt":  good + "\n",
				"sums2.txt": "junk\n",
			}),
			args:     []string{"-c", "sums.txt", "sums2.txt"},
			want:     "hello.txt: OK\n",
			wantErr:  "sha256sum: sums2.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name: "the last line may come without a newline",
			fsys: testFiles(map[string]string{"sums.txt": good}),
			args: []string{"-c", "sums.txt"},
			want: "hello.txt: OK\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunCheckLineFormats(t *testing.T) {
	good := checksumLine(helloSHA256, "hello.txt")

	tests := []testCase{
		{
			name: "a line of the standard input format",
			fsys: sumsFS(helloSHA256 + " hello.txt"),
			args: []string{"-c", "sums.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name: "a binary mode line",
			fsys: sumsFS(helloSHA256 + " *hello.txt"),
			args: []string{"-c", "sums.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name: "a BSD style line",
			fsys: sumsFS(taggedLine("SHA256", helloSHA256, "hello.txt")),
			args: []string{"-c", "sums.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name: "a BSD style line written without blanks",
			fsys: sumsFS("SHA256(hello.txt)=" + helloSHA256),
			args: []string{"-c", "sums.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name:     "a BSD style line of another algorithm",
			fsys:     sumsFS(taggedLine("MD5", helloMD5, "hello.txt")),
			args:     []string{"-c", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name:     "a BSD style line without a digest",
			fsys:     sumsFS("SHA256 (hello.txt) ="),
			args:     []string{"-c", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name: "a BSD style line naming a file with a parenthesis",
			fsys: sumsFS(taggedLine("SHA256", helloSHA256, "a)b.txt")),
			args: []string{"-c", "sums.txt"},
			want: "a)b.txt: OK\n",
		},
		{
			name:     "a BSD style line without a closing parenthesis",
			fsys:     sumsFS("SHA256 (hello.txt"),
			args:     []string{"-c", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name:     "a BSD style line without the equals sign",
			fsys:     sumsFS("SHA256 (hello.txt) " + helloSHA256),
			args:     []string{"-c", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name: "an uppercase digest",
			fsys: sumsFS(strings.ToUpper(helloSHA256) + "  hello.txt"),
			args: []string{"-c", "sums.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name: "leading blanks",
			fsys: sumsFS("\t  " + good),
			args: []string{"-c", "sums.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name: "a file name holding a leading blank",
			fsys: sumsFS(helloSHA256 + "   lead.txt"),
			args: []string{"-c", "sums.txt"},
			want: " lead.txt: OK\n",
		},
		{
			name: "CRLF line endings",
			fsys: testFiles(map[string]string{"sums.txt": good + "\r\n"}),
			args: []string{"-c", "sums.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name: "an escaped file name",
			fsys: sumsFS(`\` + helloSHA256 + `  we\nird.txt`),
			args: []string{"-c", "sums.txt"},
			want: "we\nird.txt: OK\n",
		},
		{
			name: "an escaped carriage return",
			fsys: sumsFS(`\` + helloSHA256 + `  cr\rname.txt`),
			args: []string{"-c", "sums.txt"},
			want: "cr\rname.txt: OK\n",
		},
		{
			name: "a BSD style line with an escaped file name",
			fsys: sumsFS(`\SHA256 (we\nird.txt) = ` + helloSHA256),
			args: []string{"-c", "sums.txt"},
			want: "we\nird.txt: OK\n",
		},
		{
			name:     "an unknown escape sequence",
			fsys:     sumsFS(`\` + helloSHA256 + `  we\qird.txt`),
			args:     []string{"-c", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name:     "a file name ending with a backslash",
			fsys:     sumsFS(`\` + helloSHA256 + `  broken\`),
			args:     []string{"-c", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name:     "a digest in the wrong length",
			fsys:     sumsFS(helloSHA256[:60] + "  hello.txt"),
			args:     []string{"-c", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name:     "a line which is too short",
			fsys:     sumsFS("sha256sum"),
			args:     []string{"-c", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name:     "a digest which isn't hexadecimal",
			fsys:     sumsFS(strings.Repeat("z", len(helloSHA256)) + "  hello.txt"),
			args:     []string{"-c", "sums.txt"},
			wantErr:  "sha256sum: sums.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name: "a line with a one character file name",
			fsys: sumsFS(helloSHA256 + " x"),
			args: []string{"-c", "sums.txt"},
			want: "x: FAILED open or read\n",
			wantErr: "sha256sum: x: file does not exist\n" +
				"sha256sum: WARNING: 1 listed file could not be read\n",
			wantExit: 1,
		},
		{
			name:    "the formats of the mode character can't be mixed",
			fsys:    sumsFS(good, helloSHA256+" empty.txt"),
			args:    []string{"-c", "sums.txt"},
			want:    "hello.txt: OK\n",
			wantErr: "sha256sum: WARNING: 1 line is improperly formatted\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

// TestRunCheckBackslash covers the escaped names which a Windows file system
// can't hold.
func TestRunCheckBackslash(t *testing.T) {
	runCase(t, testCase{
		name:   "an escaped backslash",
		fsys:   sumsFS(`\` + helloSHA256 + `  back\\slash.txt`),
		args:   []string{"-c", "sums.txt"},
		want:   `back\slash.txt: OK` + "\n",
		skipOn: "windows",
	})
}

func TestRunCheckStandardInput(t *testing.T) {
	good := checksumLine(helloSHA256, "hello.txt")

	tests := []testCase{
		{
			name:     "a checksum file which can't be read",
			fsys:     testFS(),
			stdin:    errorReader{},
			args:     []string{"-c"},
			wantErr:  "sha256sum: standard input: read error: boom\n",
			wantExit: 1,
		},
		{
			name:  "no operands read the checksum lines from the standard input",
			fsys:  testFS(),
			stdin: strings.NewReader(good + "\n"),
			args:  []string{"-c"},
			want:  "hello.txt: OK\n",
		},
		{
			name:  "dash reads the checksum lines from the standard input",
			fsys:  testFS(),
			stdin: strings.NewReader(good + "\n"),
			args:  []string{"-c", "-"},
			want:  "hello.txt: OK\n",
		},
		{
			name: "the standard input is named in the diagnostics",
			fsys: testFS(),
			stdin: strings.NewReader(helloSHA256 + "  missing.txt\n" +
				checksumLine(helloSHA256, "-") + "\n"),
			args: []string{"-c"},
			want: "missing.txt: FAILED open or read\n",
			wantErr: "sha256sum: missing.txt: file does not exist\n" +
				"sha256sum: WARNING: 1 line is improperly formatted\n" +
				"sha256sum: WARNING: 1 listed file could not be read\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
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
			runCase(t, testCase{
				algorithm: test.algorithm,
				fsys:      sumsFS(checksumLine(test.digest, "hello.txt")),
				args:      []string{"-c", "sums.txt"},
				want:      "hello.txt: OK\n",
			})
			// A line holding a digest which no algorithm of the family has
			// must be rejected.
			runCase(t, testCase{
				name:      "wrong length",
				algorithm: test.algorithm,
				fsys:      sumsFS(checksumLine(strings.Repeat("ab", 10), "hello.txt")),
				args:      []string{"-c", "sums.txt"},
				wantErr: test.algorithm.Name +
					": sums.txt: no properly formatted checksum lines found\n",
				wantExit: 1,
			})
		})
	}
}
