package getopt

import (
	"errors"
	"slices"
	"testing"
)

type option struct {
	opt   rune
	value string
}

func TestParse(t *testing.T) {
	tests := []struct {
		name         string
		set          Set
		args         []string
		want         []option
		wantOperands []string
		wantErr      string
	}{
		{
			name: "no arguments",
		},
		{
			name:         "operands only",
			args:         []string{"a", "b"},
			wantOperands: []string{"a", "b"},
		},
		{
			name:         "a single dash is an operand",
			args:         []string{"-", "a"},
			wantOperands: []string{"-", "a"},
		},
		{
			name:         "grouped options",
			args:         []string{"-abc", "f"},
			want:         []option{{'a', ""}, {'b', ""}, {'c', ""}},
			wantOperands: []string{"f"},
		},
		{
			name: "digits are plain options",
			args: []string{"-9"},
			want: []option{{'9', ""}},
		},
		{
			name:         "parsing stops at the first operand",
			args:         []string{"-a", "x", "-b"},
			want:         []option{{'a', ""}},
			wantOperands: []string{"x", "-b"},
		},
		{
			name:         "double dash ends the options",
			args:         []string{"-a", "--", "-b", "x"},
			want:         []option{{'a', ""}},
			wantOperands: []string{"-b", "x"},
		},
		{
			name:         "double dash is dropped",
			args:         []string{"--", "-"},
			wantOperands: []string{"-"},
		},
		{
			name: "an attached argument",
			set:  Set{Valued: "n"},
			args: []string{"-n5"},
			want: []option{{'n', "5"}},
		},
		{
			name:         "an argument in the next position",
			set:          Set{Valued: "n"},
			args:         []string{"-n", "5", "a.txt"},
			want:         []option{{'n', "5"}},
			wantOperands: []string{"a.txt"},
		},
		{
			name: "an argument takes the rest of the group",
			set:  Set{Valued: "n"},
			args: []string{"-an5b"},
			want: []option{{'a', ""}, {'n', "5b"}},
		},
		{
			name: "an empty argument is valid",
			set:  Set{Valued: "n"},
			args: []string{"-n", ""},
			want: []option{{'n', ""}},
		},
		{
			name:    "an option missing its argument",
			set:     Set{Valued: "n"},
			args:    []string{"-c", "-n"},
			want:    []option{{'c', ""}},
			wantErr: "option requires an argument -- 'n'",
		},
		{
			name: "several valued options",
			set:  Set{Valued: "cn"},
			args: []string{"-c", "5", "-n", "7"},
			want: []option{{'c', "5"}, {'n', "7"}},
		},
		{
			name:    "a long option is rejected without a table",
			args:    []string{"--all"},
			wantErr: "unrecognized option '--all'",
		},
		{
			name:    "a long option with a value is rejected without a table",
			args:    []string{"--all=yes"},
			wantErr: "unrecognized option '--all=yes'",
		},
		{
			name:         "a known long option",
			set:          Set{Long: map[string]rune{"all": 'a'}},
			args:         []string{"--all", "x"},
			want:         []option{{'a', ""}},
			wantOperands: []string{"x"},
		},
		{
			name:    "a long option which doesn't take an argument",
			set:     Set{Long: map[string]rune{"all": 'a'}},
			args:    []string{"--all=yes"},
			wantErr: "option '--all=yes' doesn't allow an argument",
		},
		{
			name: "a long option with an attached argument",
			set:  Set{Valued: "w", Long: map[string]rune{"wrap": 'w'}},
			args: []string{"--wrap=7"},
			want: []option{{'w', "7"}},
		},
		{
			name: "a long option takes the next argument",
			set:  Set{Valued: "w", Long: map[string]rune{"wrap": 'w'}},
			args: []string{"--wrap", "7"},
			want: []option{{'w', "7"}},
		},
		{
			name: "a long option takes an operand as its argument",
			set:  Set{Valued: "w", Long: map[string]rune{"wrap": 'w'}},
			args: []string{"--wrap", "x.txt"},
			want: []option{{'w', "x.txt"}},
		},
		{
			name:    "a long option missing its argument",
			set:     Set{Valued: "w", Long: map[string]rune{"wrap": 'w'}},
			args:    []string{"--wrap"},
			wantErr: "option '--wrap' requires an argument",
		},
		{
			name: "a long option with an empty argument",
			set:  Set{Valued: "w", Long: map[string]rune{"wrap": 'w'}},
			args: []string{"--wrap="},
			want: []option{{'w', ""}},
		},
		{
			name: "a long option stands for a short one",
			set:  Set{Long: map[string]rune{"all": 'a', "best": 'b', "fast": 'c'}},
			args: []string{"--all", "--best", "--fast"},
			want: []option{{'a', ""}, {'b', ""}, {'c', ""}},
		},
		{
			name:    "an unknown long option among the known ones",
			set:     Set{Long: map[string]rune{"all": 'a'}},
			args:    []string{"--all", "--bogus"},
			want:    []option{{'a', ""}},
			wantErr: "unrecognized option '--bogus'",
		},
		{
			name:         "an optional long option without an argument",
			set:          Set{Optional: "p", Long: map[string]rune{"tmpdir": 'p'}},
			args:         []string{"--tmpdir", "x"},
			want:         []option{{'p', ""}},
			wantOperands: []string{"x"},
		},
		{
			name: "an optional long option with an attached argument",
			set:  Set{Optional: "p", Long: map[string]rune{"tmpdir": 'p'}},
			args: []string{"--tmpdir=/tmp"},
			want: []option{{'p', "/tmp"}},
		},
		{
			name: "an optional long option with an empty argument",
			set:  Set{Optional: "p", Long: map[string]rune{"tmpdir": 'p'}},
			args: []string{"--tmpdir="},
			want: []option{{'p', ""}},
		},
		{
			name: "an optional short option still requires its argument",
			set:  Set{Optional: "p", Long: map[string]rune{"tmpdir": 'p'}},
			args: []string{"-p", "dir"},
			want: []option{{'p', "dir"}},
		},
		{
			name: "an optional short option takes the rest of the group",
			set:  Set{Optional: "p", Long: map[string]rune{"tmpdir": 'p'}},
			args: []string{"-qp", "dir"},
			want: []option{{'q', ""}, {'p', "dir"}},
		},
		{
			name:    "an optional short option missing its argument",
			set:     Set{Optional: "p", Long: map[string]rune{"tmpdir": 'p'}},
			args:    []string{"-p"},
			wantErr: "option requires an argument -- 'p'",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got []option
			operands, err := test.set.Parse(test.args, func(opt rune, value string) error {
				got = append(got, option{opt, value})
				return nil
			})

			if !slices.Equal(got, test.want) {
				t.Errorf("options = %v, want %v", got, test.want)
			}
			if !slices.Equal(operands, test.wantOperands) {
				t.Errorf("operands = %v, want %v", operands, test.wantOperands)
			}
			switch {
			case test.wantErr == "" && err != nil:
				t.Errorf("err = %v, want none", err)
			case test.wantErr != "" && err == nil:
				t.Errorf("err = none, want %q", test.wantErr)
			case test.wantErr != "" && err.Error() != test.wantErr:
				t.Errorf("err = %q, want %q", err, test.wantErr)
			}
		})
	}
}

func TestParseVisitError(t *testing.T) {
	sentinel := errors.New("no such file")
	var got []rune
	operands, err := Set{}.Parse([]string{"-a", "-b", "x"}, func(opt rune, _ string) error {
		got = append(got, opt)
		if opt == 'b' {
			return sentinel
		}
		return nil
	})

	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want %v", err, sentinel)
	}
	if !slices.Equal(got, []rune{'a', 'b'}) {
		t.Errorf("options = %v, want a and b only", got)
	}
	if operands != nil {
		t.Errorf("operands = %v, want none", operands)
	}
}

func TestInvalidOption(t *testing.T) {
	if got := InvalidOption('x').Error(); got != "invalid option -- 'x'" {
		t.Errorf("error = %q, want %q", got, "invalid option -- 'x'")
	}
}
