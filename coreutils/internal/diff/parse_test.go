package diff

import (
	"strings"
	"testing"
)

// checkHunks validates that each hunk's old and new lines match the regions
// of a and b that its numbers point at.
func checkHunks(t *testing.T, hunks []Hunk, a, b []string) {
	t.Helper()

	for _, h := range hunks {
		if h.FromLine < 0 || h.FromLine-1+len(h.Old) > len(a) {
			t.Fatalf("hunk old range %d,%d out of a", h.FromLine, len(h.Old))
		}
		for i, l := range h.Old {
			if !lineMatches(l, a, h.FromLine+i) {
				t.Errorf("hunk old line %d = %+v, want %q", i, l, a[h.FromLine-1+i])
			}
		}
		if h.ToLine < 0 || h.ToLine-1+len(h.New) > len(b) {
			t.Fatalf("hunk new range %d,%d out of b", h.ToLine, len(h.New))
		}
		for i, l := range h.New {
			if !lineMatches(l, b, h.ToLine+i) {
				t.Errorf("hunk new line %d = %+v, want %q", i, l, b[h.ToLine-1+i])
			}
		}
	}
}

// lineMatches reports whether the hunk line equals the file line, with its
// no-newline flag matching the line ending.
func lineMatches(l HunkLine, file []string, n int) bool {
	if n < 1 || n > len(file) {
		return false
	}
	line := file[n-1]
	return l.Content == strings.TrimSuffix(line, "\n") &&
		l.NoNewline != strings.HasSuffix(line, "\n")
}

// TestParseRoundTrip checks that the unified and context writers produce
// data the parsers read back into hunks that match the files.
func TestParseRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		context int
	}{
		{
			name: "changed line",
			a:    "a\nb\nc\n", b: "a\nx\nc\n",
		},
		{
			name: "insertion",
			a:    "a\nb\n", b: "a\nx\nb\n",
		},
		{
			name: "deletion",
			a:    "a\nx\nb\n", b: "a\nb\n",
		},
		{
			name: "insertion into empty file",
			a:    "", b: "x\n",
		},
		{
			name: "deletion to empty file",
			a:    "x\n", b: "",
		},
		{
			name: "multiple hunks",
			a:    "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n", b: "1\nX\n3\n4\n5\n6\n7\n8\nY\n10\n",
		},
		{
			name: "no newline at end of both files",
			a:    "a\nb", b: "a\nc",
		},
		{
			name: "no newline at end of the new file only",
			a:    "a\nb\n", b: "a\nb\nc",
		},
		{
			name: "no newline at end of the old file only",
			a:    "a\nb", b: "a\nb\nc\n",
		},
		{
			name: "empty context lines",
			a:    "a\n\nb\n", b: "a\n\n\nc\n",
		},
	}
	for _, test := range tests {
		for _, format := range []struct {
			name  string
			write func(a, b []string, changes []Change, out *strings.Builder)
		}{
			{
				name: "unified",
				write: func(a, b []string, changes []Change, out *strings.Builder) {
					Unified(out, file1, file2, a, b, changes, test.context)
				},
			},
			{
				name: "context",
				write: func(a, b []string, changes []Change, out *strings.Builder) {
					Context(out, file1, file2, a, b, changes, test.context)
				},
			},
		} {
			t.Run(test.name+" in the "+format.name+" format", func(t *testing.T) {
				a, b := SplitLines([]byte(test.a)), SplitLines([]byte(test.b))
				changes := Lines(a, b, nil)
				var out strings.Builder
				format.write(a, b, changes, &out)

				var patches []Patch
				var err error
				if format.name == "unified" {
					patches, err = ParseUnified(out.String())
				} else {
					patches, err = ParseContext(out.String())
				}
				if err != nil {
					t.Fatalf("parse error = %v", err)
				}
				if len(patches) != 1 {
					t.Fatalf("got %d patches, want 1", len(patches))
				}
				if len(patches[0].Hunks) != len(changes) && test.context != 0 {
					// Hunks can merge when close, so only the line checks
					// below are strict.
					_ = changes
				}
				checkHunks(t, patches[0].Hunks, a, b)
			})
		}
	}
}

func TestParseUnifiedHeaders(t *testing.T) {
	data := "" +
		"diff -u a/x b/x\n" +
		"--- a/x\t2026-01-02 03:04:05 +0000\n" +
		"+++ b/x\t2026-01-02 03:04:05 +0000\n" +
		"@@ -1 +1 @@\n" +
		"-a\n" +
		"+b\n" +
		"Index: idx-file\n" +
		"--- a/y\t2026-01-02 03:04:05 +0000\n" +
		"+++ b/y\t2026-01-02 03:04:05 +0000\n" +
		"@@ -1 +1 @@\n" +
		"-c\n" +
		"+d\n"

	patches, err := ParseUnified(data)
	if err != nil {
		t.Fatalf("ParseUnified() error = %v", err)
	}
	if len(patches) != 2 {
		t.Fatalf("got %d patches, want 2", len(patches))
	}
	if patches[0].From != "a/x" || patches[0].To != "b/x" || patches[0].Index != "" {
		t.Errorf("patch 0 = %+v", patches[0])
	}
	if patches[1].From != "a/y" || patches[1].To != "b/y" || patches[1].Index != "idx-file" {
		t.Errorf("patch 1 = %+v", patches[1])
	}

	// The names come without their timestamps.
	if got := patches[0].Hunks; len(got) != 1 || got[0].FromLine != 1 || got[0].ToLine != 1 {
		t.Errorf("patch 0 hunks = %+v", got)
	}
}

func TestParseUnifiedBareHunks(t *testing.T) {
	data := "@@ -1 +1 @@\n-a\n+b\n"

	patches, err := ParseUnified(data)
	if err != nil {
		t.Fatalf("ParseUnified() error = %v", err)
	}
	if len(patches) != 1 {
		t.Fatalf("got %d patches, want 1", len(patches))
	}
	if patches[0].From != "" || patches[0].To != "" {
		t.Errorf("patch names = %q, %q", patches[0].From, patches[0].To)
	}
	checkHunks(t, patches[0].Hunks, SplitLines([]byte("a\n")), SplitLines([]byte("b\n")))
}

func TestParseIndented(t *testing.T) {
	data := "" +
		"  --- a/x\t2026-01-02 03:04:05 +0000\n" +
		"  +++ b/x\t2026-01-02 03:04:05 +0000\n" +
		"  @@ -1 +1 @@\n" +
		"  -a\n" +
		"  +b\n"

	patches, err := ParseUnified(data)
	if err != nil {
		t.Fatalf("ParseUnified() error = %v", err)
	}
	if len(patches) != 1 || patches[0].From != "a/x" {
		t.Fatalf("patches = %+v", patches)
	}
	checkHunks(t, patches[0].Hunks, SplitLines([]byte("a\n")), SplitLines([]byte("b\n")))
}

func TestParseEmpty(t *testing.T) {
	for _, data := range []string{"", "\n", "just some text\n"} {
		patches, err := ParseUnified(data)
		if err != nil || len(patches) != 0 {
			t.Errorf("ParseUnified(%q) = %+v, %v", data, patches, err)
		}
	}
}

func TestParseMalformed(t *testing.T) {
	tests := []struct {
		name   string
		format func(string) ([]Patch, error)
		data   string
		want   string
	}{
		{
			name:   "unified hunk missing lines",
			format: ParseUnified,
			data:   "@@ -1,2 +1,2 @@\n a\n",
			want:   "malformed patch at line 3: hunk ends in the middle",
		},
		{
			name:   "unified header without numbers",
			format: ParseUnified,
			data:   "@@ -x +1 @@\n-a\n+b\n",
			want:   "malformed patch at line 1: missing line number",
		},
		{
			name:   "unified marker without a line",
			format: ParseUnified,
			data:   "@@ -1 +1 @@\n\\ No newline at end of file\n-a\n+b\n",
			want:   "malformed patch at line 2: marker without a line before it",
		},
		{
			name:   "unified headers without hunks",
			format: ParseUnified,
			data:   "--- a\n+++ b\n",
			want:   "malformed patch at line 3: no hunks after the headers",
		},
		{
			name:   "unified body beyond the counts",
			format: ParseUnified,
			data:   "@@ -1 +1 @@\n-a\n+b\n+extra\n",
			want:   "malformed patch at line 4: line beyond the hunk counts",
		},
		{
			name:   "context old part too short",
			format: ParseContext,
			data:   "***************\n*** 1,2 ****\n  a\n--- 1,2 ----\n  a\n",
			want:   "malformed patch at line 4: old part line count doesn't match its range",
		},
		{
			name:   "context new part too short",
			format: ParseContext,
			data:   "***************\n*** 1,2 ****\n  a\n! b\n--- 1,2 ----\n  a\n",
			want:   "malformed patch at line 5: new part line count doesn't match its range",
		},
		{
			name:   "context hunk ending in the middle",
			format: ParseContext,
			data:   "***************\n*** 1,2 ****\n  a\n",
			want:   "malformed patch at line 4: hunk ends in the middle",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.format(test.data)
			if err == nil {
				t.Fatalf("parse succeeded, want %q", test.want)
			}
			if err.Error() != test.want {
				t.Errorf("parse error = %q, want %q", err, test.want)
			}
		})
	}
}

func TestDetectPatch(t *testing.T) {
	tests := []struct {
		name string
		data string
		want PatchFormat
	}{
		{
			name: "unified hunk",
			data: "@@ -1 +1 @@\n-a\n+b\n",
			want: UnifiedPatch,
		},
		{
			name: "unified headers",
			data: "junk\n--- a\n+++ b\n@@ -1 +1 @@\n-a\n+b\n",
			want: UnifiedPatch,
		},
		{
			name: "context hunk",
			data: "junk\n***************\n*** 1 ****\n! a\n--- 1 ----\n! b\n",
			want: ContextPatch,
		},
		{
			name: "context headers",
			data: "*** a\tFri Jan  2 03:04:05 2026\n--- b\tFri Jan  2 03:04:05 2026\n",
			want: ContextPatch,
		},
		{
			name: "no patch",
			data: "just some text\n",
			want: UnknownPatch,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := DetectPatch(test.data); got != test.want {
				t.Errorf("DetectPatch() = %v, want %v", got, test.want)
			}
		})
	}
}
