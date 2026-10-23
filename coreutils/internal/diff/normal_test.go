package diff

import (
	"strings"
	"testing"
)

func TestNormal(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want string
	}{
		{
			name: "no changes",
			a:    "a\nb\n", b: "a\nb\n",
		},
		{
			name: "changed line",
			a:    "a\nb\nc\n", b: "a\nx\nc\n",
			want: "2c2\n< b\n---\n> x\n",
		},
		{
			name: "changed line into a range",
			a:    "a\nb\nc\n", b: "a\nx\ny\nc\n",
			want: "2c2,3\n< b\n---\n> x\n> y\n",
		},
		{
			name: "insertion at the start",
			a:    "a\n", b: "x\na\n",
			want: "0a1\n> x\n",
		},
		{
			name: "insertion in the middle",
			a:    "a\nb\n", b: "a\nx\nb\n",
			want: "1a2\n> x\n",
		},
		{
			name: "insertion of a range at the end",
			a:    "a\n", b: "a\nx\ny\n",
			want: "1a2,3\n> x\n> y\n",
		},
		{
			name: "deletion at the start",
			a:    "x\na\n", b: "a\n",
			want: "1d0\n< x\n",
		},
		{
			name: "deletion in the middle",
			a:    "a\nx\nb\n", b: "a\nb\n",
			want: "2d1\n< x\n",
		},
		{
			name: "deletion of a range at the end",
			a:    "a\nx\ny\n", b: "a\n",
			want: "2,3d1\n< x\n< y\n",
		},
		{
			name: "replacement of a range",
			a:    "a\nb\nc\nd\n", b: "a\nx\ny\n",
			want: "2,4c2,3\n< b\n< c\n< d\n---\n> x\n> y\n",
		},
		{
			name: "missing newline at end of file",
			a:    "a\nb", b: "a\nc",
			want: "2c2\n< b\n\\ No newline at end of file\n---\n> c\n\\ No newline at end of file\n",
		},
		{
			name: "missing newline on one side only",
			a:    "a\nb\n", b: "a\nb\nc",
			want: "2a3\n> c\n\\ No newline at end of file\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, b := SplitLines([]byte(test.a)), SplitLines([]byte(test.b))
			changes := Lines(a, b, nil)
			var out strings.Builder
			if err := Normal(&out, a, b, changes); err != nil {
				t.Fatalf("Normal() error = %v", err)
			}
			if got := out.String(); got != test.want {
				t.Errorf("Normal() = %q, want %q", got, test.want)
			}
		})
	}
}
