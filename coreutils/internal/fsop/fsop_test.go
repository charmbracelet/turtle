package fsop

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
)

func TestSplitOperands(t *testing.T) {
	tests := []struct {
		name         string
		operands     []string
		targetDir    string
		targetDirSet bool
		noTarget     bool
		wantSources  []string
		wantTarget   string
		wantErr      string
	}{
		{
			name:       "no operands",
			operands:   nil,
			wantTarget: "",
			wantErr:    "missing file operand",
		},
		{
			name:     "one operand",
			operands: []string{"a"},
			wantErr:  "missing destination file operand after 'a'",
		},
		{
			name:        "two operands",
			operands:    []string{"a", "b"},
			wantSources: []string{"a"},
			wantTarget:  "b",
		},
		{
			name:        "many operands",
			operands:    []string{"a", "b", "c", "d"},
			wantSources: []string{"a", "b", "c"},
			wantTarget:  "d",
		},
		{
			name:         "target directory",
			operands:     []string{"a", "b"},
			targetDir:    "dir",
			targetDirSet: true,
			wantSources:  []string{"a", "b"},
			wantTarget:   "dir",
		},
		{
			name:         "target directory without operands",
			operands:     nil,
			targetDir:    "dir",
			targetDirSet: true,
			wantErr:      "missing file operand",
		},
		{
			name:        "no target with two operands",
			operands:    []string{"a", "b"},
			noTarget:    true,
			wantSources: []string{"a"},
			wantTarget:  "b",
		},
		{
			name:     "no target with extra operand",
			operands: []string{"a", "b", "c"},
			noTarget: true,
			wantErr:  "extra operand 'c'",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sources, target, err := SplitOperands(test.operands, test.targetDir, test.targetDirSet, test.noTarget)
			var msg string
			if err != nil {
				msg = err.Error()
			}
			if msg != test.wantErr {
				t.Fatalf("SplitOperands() error = %q, want %q", msg, test.wantErr)
			}
			if err != nil {
				return
			}
			if !reflect.DeepEqual(sources, test.wantSources) {
				t.Errorf("SplitOperands() sources = %q, want %q", sources, test.wantSources)
			}
			if target != test.wantTarget {
				t.Errorf("SplitOperands() target = %q, want %q", target, test.wantTarget)
			}
		})
	}
}

func TestPairChild(t *testing.T) {
	e := Pair{Src: "src", Dst: "dst", SrcName: "s", DstName: "d"}
	want := Pair{Src: "src/a", Dst: "dst/a", SrcName: "s/a", DstName: "d/a"}
	if got := e.Child("a"); got != want {
		t.Errorf("Child() = %+v, want %+v", got, want)
	}
}

func TestConfirm(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "yes", in: "y\n", want: true},
		{name: "uppercase yes", in: "Yes\n", want: true},
		{name: "no", in: "n\n"},
		{name: "empty line", in: "\ny\n"},
		{name: "answer only", in: "y", want: true},
		{name: "none given", in: ""},
		{name: "first byte decides", in: "ny\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			rt := &coreutils.Runtime{Stdin: strings.NewReader(test.in), Stderr: &stderr}
			if got := Confirm(rt, "cp", "overwrite", "dst"); got != test.want {
				t.Errorf("Confirm() = %v, want %v", got, test.want)
			}
			if want := "cp: overwrite 'dst'? "; stderr.String() != want {
				t.Errorf("prompt = %q, want %q", stderr.String(), want)
			}
		})
	}

	t.Run("null standard input", func(t *testing.T) {
		var stderr bytes.Buffer
		rt := &coreutils.Runtime{Stderr: &stderr}
		if Confirm(rt, "cp", "overwrite", "dst") {
			t.Error("Confirm() with nil stdin = true, want false")
		}
		if want := "cp: overwrite 'dst'? "; stderr.String() != want {
			t.Errorf("prompt = %q, want %q", stderr.String(), want)
		}
	})
}
