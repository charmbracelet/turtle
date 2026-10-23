package diff

import (
	"strings"
	"testing"
	"time"
)

var (
	stamp1 = time.Date(2026, 10, 23, 15, 41, 23, 0, time.UTC)
	file1  = File{Name: "f1", Time: stamp1}
	file2  = File{Name: "f2", Time: stamp1}
)

func TestContext(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		context int
		want    string
	}{
		{
			name: "changed line",
			a:    "a\nb\nc\n", b: "a\nx\nc\n",
			context: 3,
			want: "***************\n*** 1,3 ****\n  a\n! b\n  c\n" +
				"--- 1,3 ----\n  a\n! x\n  c\n",
		},
		{
			name: "single line files",
			a:    "a\n", b: "b\n",
			context: 3,
			want: "***************\n*** 1 ****\n! a\n" +
				"--- 1 ----\n! b\n",
		},
		{
			name: "insertion into empty file",
			a:    "", b: "x\n",
			context: 3,
			want: "***************\n*** 0 ****\n" +
				"--- 1 ----\n+ x\n",
		},
		{
			name: "deletion to one line",
			a:    "a\nb\n", b: "a\n",
			context: 3,
			want: "***************\n*** 1,2 ****\n  a\n- b\n" +
				"--- 1 ----\n  a\n",
		},
		{
			name: "insertion in the middle",
			a:    "a\nb\nc\n", b: "a\nx\nb\nc\n",
			context: 3,
			want: "***************\n*** 1,3 ****\n  a\n  b\n  c\n" +
				"--- 1,4 ----\n  a\n+ x\n  b\n  c\n",
		},
		{
			name: "changes merged into one hunk",
			a:    "1\n2\n3\n4\n5\n6\n7\n", b: "1\nX\n3\n4\nY\n6\n7\n",
			context: 1,
			want: "***************\n*** 1,6 ****\n  1\n! 2\n  3\n  4\n! 5\n  6\n" +
				"--- 1,6 ----\n  1\n! X\n  3\n  4\n! Y\n  6\n",
		},
		{
			name: "changes kept apart",
			a:    "1\n2\n3\n4\n5\n6\n7\n", b: "1\nX\n3\n4\n5\nY\n7\n",
			context: 1,
			want: "***************\n*** 1,3 ****\n  1\n! 2\n  3\n" +
				"--- 1,3 ----\n  1\n! X\n  3\n" +
				"***************\n*** 5,7 ****\n  5\n! 6\n  7\n" +
				"--- 5,7 ----\n  5\n! Y\n  7\n",
		},
		{
			name: "missing newline at end of file",
			a:    "a\nb", b: "a\nc",
			context: 3,
			want: "***************\n*** 1,2 ****\n  a\n! b\n\\ No newline at end of file\n" +
				"--- 1,2 ----\n  a\n! c\n\\ No newline at end of file\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, b := SplitLines([]byte(test.a)), SplitLines([]byte(test.b))
			changes := Lines(a, b, nil)
			var out strings.Builder
			if err := Context(&out, file1, file2, a, b, changes, test.context); err != nil {
				t.Fatalf("Context() error = %v", err)
			}
			want := "*** f1\tFri Oct 23 15:41:23 2026\n--- f2\tFri Oct 23 15:41:23 2026\n" + test.want
			if got := out.String(); got != want {
				t.Errorf("Context() = %q, want %q", got, want)
			}
		})
	}
}

func TestUnified(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		context int
		want    string
	}{
		{
			name: "changed line",
			a:    "a\nb\nc\n", b: "a\nx\nc\n",
			context: 3,
			want:    "@@ -1,3 +1,3 @@\n a\n-b\n+x\n c\n",
		},
		{
			name: "single line files",
			a:    "a\n", b: "b\n",
			context: 3,
			want:    "@@ -1 +1 @@\n-a\n+b\n",
		},
		{
			name: "insertion into empty file",
			a:    "", b: "x\n",
			context: 3,
			want:    "@@ -0,0 +1 @@\n+x\n",
		},
		{
			name: "deletion of the last line",
			a:    "a\nb\n", b: "a\n",
			context: 3,
			want:    "@@ -1,2 +1 @@\n a\n-b\n",
		},
		{
			name: "insertion in the middle",
			a:    "a\nb\nc\n", b: "a\nx\nb\nc\n",
			context: 3,
			want:    "@@ -1,3 +1,4 @@\n a\n+x\n b\n c\n",
		},
		{
			name: "changes merged into one hunk",
			a:    "1\n2\n3\n4\n5\n6\n7\n", b: "1\nX\n3\n4\nY\n6\n7\n",
			context: 1,
			want:    "@@ -1,6 +1,6 @@\n 1\n-2\n+X\n 3\n 4\n-5\n+Y\n 6\n",
		},
		{
			name: "changes kept apart",
			a:    "1\n2\n3\n4\n5\n6\n7\n", b: "1\nX\n3\n4\n5\nY\n7\n",
			context: 1,
			want: "@@ -1,3 +1,3 @@\n 1\n-2\n+X\n 3\n" +
				"@@ -5,3 +5,3 @@\n 5\n-6\n+Y\n 7\n",
		},
		{
			name: "insertion without context",
			a:    "1\n2\n3\n", b: "1\n2\nX\n3\n",
			context: 0,
			want:    "@@ -2,0 +3 @@\n+X\n",
		},
		{
			name: "missing newline at end of file",
			a:    "a\nb", b: "a\nc",
			context: 3,
			want:    "@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+c\n\\ No newline at end of file\n",
		},
		{
			name: "missing newline on one side only",
			a:    "a\nb\n", b: "a\nb\nc",
			context: 3,
			want:    "@@ -1,2 +1,3 @@\n a\n b\n+c\n\\ No newline at end of file\n",
		},
		{
			name: "changed first line without newline",
			a:    "b", b: "c",
			context: 3,
			want:    "@@ -1 +1 @@\n-b\n\\ No newline at end of file\n+c\n\\ No newline at end of file\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, b := SplitLines([]byte(test.a)), SplitLines([]byte(test.b))
			changes := Lines(a, b, nil)
			var out strings.Builder
			if err := Unified(&out, file1, file2, a, b, changes, test.context); err != nil {
				t.Fatalf("Unified() error = %v", err)
			}
			want := "--- f1\t2026-10-23 15:41:23 +0000\n+++ f2\t2026-10-23 15:41:23 +0000\n" + test.want
			if got := out.String(); got != want {
				t.Errorf("Unified() = %q, want %q", got, want)
			}
		})
	}
}

func TestEd(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want string
	}{
		{
			name: "changed lines",
			a:    "a\n.\nb\n", b: "x\n.\ny\n",
			want: "3c\ny\n.\n1c\nx\n.\n",
		},
		{
			name: "changed range",
			a:    "1\n2\n3\n4\n5\n", b: "1\nx\n4\n5\n",
			want: "2,3c\nx\n.\n",
		},
		{
			name: "insertion",
			a:    "a\nb\n", b: "a\nx\nb\n",
			want: "1a\nx\n.\n",
		},
		{
			name: "insertion at the start",
			a:    "", b: "q\n",
			want: "0a\nq\n.\n",
		},
		{
			name: "deletion",
			a:    "a\nx\nb\n", b: "a\nb\n",
			want: "2d\n",
		},
		{
			name: "lone period replaced",
			a:    "a\n", b: ".\n",
			want: "1c\n..\n.\n1s/.//\n",
		},
		{
			name: "lone period inserted",
			a:    "a\nb\n", b: "a\n.\nb\n",
			want: "1a\n..\n.\n2s/.//\n",
		},
		{
			name: "lone periods replaced in a range",
			a:    "a\nb\n", b: ".\n.\n",
			want: "1,2c\n..\n..\n.\n1s/.//\n2s/.//\n",
		},
		{
			name: "missing newline at end of file",
			a:    "a\nb", b: "a\nc",
			want: "2c\nc\n.\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := SplitLines([]byte(test.b))
			changes := Lines(SplitLines([]byte(test.a)), b, nil)
			var out strings.Builder
			if err := Ed(&out, b, changes); err != nil {
				t.Fatalf("Ed() error = %v", err)
			}
			if got := out.String(); got != test.want {
				t.Errorf("Ed() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestForwardEd(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want string
	}{
		{
			name: "changed lines",
			a:    "a\n.\nb\n", b: "x\n.\ny\n",
			want: "c1\nx\n.\nc3\ny\n.\n",
		},
		{
			name: "changed range",
			a:    "1\n2\n3\n4\n5\n", b: "1\nx\n4\n5\n",
			want: "c2 3\nx\n.\n",
		},
		{
			name: "insertion",
			a:    "a\nb\n", b: "a\nx\nb\n",
			want: "a1\nx\n.\n",
		},
		{
			name: "insertion at the start",
			a:    "", b: "q\n",
			want: "a0\nq\n.\n",
		},
		{
			name: "deletion",
			a:    "a\nx\nb\n", b: "a\nb\n",
			want: "d2\n",
		},
		{
			name: "lone period inserted as is",
			a:    "a\nb\n", b: "a\n.\nb\n",
			want: "a1\n.\n.\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := SplitLines([]byte(test.b))
			changes := Lines(SplitLines([]byte(test.a)), b, nil)
			var out strings.Builder
			if err := ForwardEd(&out, b, changes); err != nil {
				t.Fatalf("ForwardEd() error = %v", err)
			}
			if got := out.String(); got != test.want {
				t.Errorf("ForwardEd() = %q, want %q", got, test.want)
			}
		})
	}
}
