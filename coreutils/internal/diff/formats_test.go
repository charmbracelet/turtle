package diff

import (
	"strings"
	"testing"
)

func TestRCS(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want string
	}{
		{
			name: "changed line",
			a:    "a\nb\nc\n", b: "a\nx\nc\n",
			want: "d2 1\na2 1\nx\n",
		},
		{
			name: "deleted line",
			a:    "a\nb\nc\n", b: "a\nc\n",
			want: "d2 1\n",
		},
		{
			name: "added line",
			a:    "a\nc\n", b: "a\nb\nc\n",
			want: "a1 1\nb\n",
		},
		{
			name: "changed range",
			a:    "1\n2\n3\n", b: "1\nx\ny\n3\n",
			want: "d2 1\na2 2\nx\ny\n",
		},
		{
			name: "insertion into empty",
			a:    "", b: "x\n",
			want: "a0 1\nx\n",
		},
		{
			name: "deletion to empty",
			a:    "x\n", b: "",
			want: "d1 1\n",
		},
		{
			name: "two changes",
			a:    "1\n2\n3\n4\n5\n", b: "1\nX\n3\n4\nY\n",
			want: "d2 1\na2 1\nX\nd5 1\na5 1\nY\n",
		},
		{
			name: "incomplete line ends incomplete",
			a:    "a\nb\n", b: "a\nx",
			want: "d2 1\na2 1\nx",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, b := SplitLines([]byte(test.a)), SplitLines([]byte(test.b))
			changes := Lines(a, b, nil)
			var out strings.Builder
			if err := RCS(&out, b, changes); err != nil {
				t.Fatalf("RCS() error = %v", err)
			}
			if got := out.String(); got != test.want {
				t.Errorf("RCS() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestIfdef(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want string
	}{
		{
			name: "changed line",
			a:    "a\nb\nc\n", b: "a\nx\nc\n",
			want: "a\n#ifndef NAME\nb\n#else /* NAME */\nx\n#endif /* NAME */\nc\n",
		},
		{
			name: "inserted lines",
			a:    "a\n", b: "a\nx\ny\n",
			want: "a\n#ifdef NAME\nx\ny\n#endif /* NAME */\n",
		},
		{
			name: "deleted lines",
			a:    "a\nx\ny\n", b: "a\n",
			want: "a\n#ifndef NAME\nx\ny\n#endif /* ! NAME */\n",
		},
		{
			name: "two changes",
			a:    "1\n2\n3\n4\n", b: "1\nX\n3\nY\n",
			want: "1\n#ifndef NAME\n2\n#else /* NAME */\nX\n#endif /* NAME */\n3\n" +
				"#ifndef NAME\n4\n#else /* NAME */\nY\n#endif /* NAME */\n",
		},
		{
			name: "incomplete line is completed",
			a:    "a\nb", b: "a\nc",
			want: "a\n#ifndef NAME\nb\n#else /* NAME */\nc\n#endif /* NAME */\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, b := SplitLines([]byte(test.a)), SplitLines([]byte(test.b))
			changes := Lines(a, b, nil)
			var out strings.Builder
			if err := Ifdef(&out, "NAME", a, b, changes); err != nil {
				t.Fatalf("Ifdef() error = %v", err)
			}
			if got := out.String(); got != test.want {
				t.Errorf("Ifdef() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSide(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		opts SideOpts
		want string
	}{
		{
			name: "narrow width puts the marks in a tight gutter",
			a:    "a\nx\n", b: "a\ny\n",
			opts: SideOpts{Width: 10},
			want: "a\ta\nx   |\ty\n",
		},
		{
			name: "deleted line keeps the left column alone",
			a:    "a\nx\n", b: "a\n",
			opts: SideOpts{Width: 10},
			want: "a\ta\nx   <\n",
		},
		{
			name: "added line keeps the right column alone",
			a:    "a\n", b: "a\ny\n",
			opts: SideOpts{Width: 10},
			want: "a\ta\n    >\ty\n",
		},
		{
			name: "a changed pair which differs in newline gets a slash",
			a:    "a\nx\n", b: "a\ny",
			opts: SideOpts{Width: 10},
			want: "a\ta\nx   /\ty\n",
		},
		{
			name: "empty right line prints no gutter",
			a:    "a\n", b: "a\n\n",
			opts: SideOpts{Width: 10},
			want: "a\ta\n    >\n",
		},
		{
			name: "left column only",
			a:    "a\nx\n", b: "a\nx\n",
			opts: SideOpts{Width: 10, LeftColumn: true},
			want: "a   (\nx   (\n",
		},
		{
			name: "suppressed common lines",
			a:    "a\nx\n", b: "a\ny\n",
			opts: SideOpts{Width: 10, SuppressCommon: true},
			want: "x   |\ty\n",
		},
		{
			name: "long lines are truncated to the column",
			a:    "0123456789x\n", b: "0123456789y\n",
			opts: SideOpts{Width: 12},
			want: "0123 |\t0123\n",
		},
		{
			name: "text which doesn't fit its column is dropped",
			a:    "\t\n", b: "\tx\n",
			opts: SideOpts{Width: 10},
			want: "    |\t\n",
		},
		{
			name: "tabs expand with -t",
			a:    "a\nb\tc\n", b: "a\nb\td\n",
			opts: SideOpts{Width: 24, ExpandTabs: true},
			want: "a             a\nb       c  |  b       d\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, b := SplitLines([]byte(test.a)), SplitLines([]byte(test.b))
			changes := Lines(a, b, nil)
			var out strings.Builder
			if err := Side(&out, a, b, changes, test.opts); err != nil {
				t.Fatalf("Side() error = %v", err)
			}
			if got := out.String(); got != test.want {
				t.Errorf("Side() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSideIdentical(t *testing.T) {
	// The side-by-side format writes the common lines even when the
	// files are equal, as the whole files come out in two columns.
	a := SplitLines([]byte("a\nb\n"))
	var out strings.Builder
	if err := Side(&out, a, a, nil, SideOpts{Width: 10}); err != nil {
		t.Fatalf("Side() error = %v", err)
	}
	if want := "a\ta\nb\tb\n"; out.String() != want {
		t.Errorf("Side() = %q, want %q", out.String(), want)
	}
}

func TestSideColumns(t *testing.T) {
	tests := []struct {
		width, tabSize int
		expand         bool
		hw, c2o        int
	}{
		{width: 130, tabSize: 8, hw: 61, c2o: 64},
		{width: 40, tabSize: 8, hw: 16, c2o: 24},
		{width: 130, tabSize: 8, expand: true, hw: 63, c2o: 67},
		{width: 10, tabSize: 8, hw: 2, c2o: 8},
	}
	for _, test := range tests {
		hw, c2o := sideColumns(SideOpts{Width: test.width, TabSize: test.tabSize, ExpandTabs: test.expand})
		if hw != test.hw || c2o != test.c2o {
			t.Errorf("sideColumns(width=%d) = (%d, %d), want (%d, %d)", test.width, hw, c2o, test.hw, test.c2o)
		}
	}
}
