package expr

import (
	"bytes"
	"errors"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
)

// runExpr evaluates args with a fresh runtime, returning the standard
// output, the standard error and the error of the run.
func runExpr(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
	err := New(rt).Run(t.Context(), args...)
	return stdout.String(), stderr.String(), err
}

func TestArithmetic(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
	}{
		{
			name:    "adds",
			args:    []string{"1", "+", "1"},
			wantOut: "2\n",
		},
		{
			name:    "subtracts into negative",
			args:    []string{"3", "-", "5"},
			wantOut: "-2\n",
		},
		{
			name:    "multiplies",
			args:    []string{"2", "*", "3"},
			wantOut: "6\n",
		},
		{
			name:    "divides, truncating toward zero",
			args:    []string{"10", "/", "3"},
			wantOut: "3\n",
		},
		{
			name:    "divides a negative, truncating toward zero",
			args:    []string{"-10", "/", "3"},
			wantOut: "-3\n",
		},
		{
			name:    "gives the remainder of a division",
			args:    []string{"10", "%", "3"},
			wantOut: "1\n",
		},
		{
			name:    "gives the remainder of a negative dividend",
			args:    []string{"-10", "%", "3"},
			wantOut: "-1\n",
		},
		{
			name:    "multiplies before adding",
			args:    []string{"1", "+", "2", "*", "3"},
			wantOut: "7\n",
		},
		{
			name:    "is left-associative on subtraction",
			args:    []string{"10", "-", "3", "-", "2"},
			wantOut: "5\n",
		},
		{
			name:    "is left-associative on division",
			args:    []string{"100", "/", "10", "/", "2"},
			wantOut: "5\n",
		},
		{
			name:    "mixes precedence levels",
			args:    []string{"2", "*", "3", "+", "4", "*", "5"},
			wantOut: "26\n",
		},
		{
			name:    "drops leading zeros when converting an operand",
			args:    []string{"007", "+", "0"},
			wantOut: "7\n",
		},
		{
			name:    "keeps leading zeros of a bare operand",
			args:    []string{"007"},
			wantOut: "007\n",
		},
		{
			name:    "sums factors up to the top of the 64-bit range",
			args:    []string{"4611686018427387903", "*", "2"},
			wantOut: "9223372036854775806\n",
		},
		{
			name:    "multiplies a negative by a positive",
			args:    []string{"-2", "*", "3"},
			wantOut: "-6\n",
		},
		{
			name:    "multiplies two negatives",
			args:    []string{"-2", "*", "-3"},
			wantOut: "6\n",
		},
		{
			name:    "multiplies the lowest integer by one",
			args:    []string{"-9223372036854775808", "*", "1"},
			wantOut: "-9223372036854775808\n",
		},
		{
			name:    "multiplies the lowest integer by a positive one",
			args:    []string{"-4611686018427387904", "*", "2"},
			wantOut: "-9223372036854775808\n",
		},
		{
			name:     "multiplies by zero",
			args:     []string{"0", "*", "-5"},
			wantOut:  "0\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, _, err := runExpr(t, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if out != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", out, test.wantOut)
			}
		})
	}

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name     string
			args     []string
			wantErr  string
			wantCode int
		}{
			{
				name:     "division by zero",
				args:     []string{"1", "/", "0"},
				wantErr:  "expr: division by zero\n",
				wantCode: 2,
			},
			{
				name:     "modulo by zero",
				args:     []string{"1", "%", "0"},
				wantErr:  "expr: division by zero\n",
				wantCode: 2,
			},
			{
				name:     "non-integer operand",
				args:     []string{"1", "+", "a"},
				wantErr:  "expr: non-integer argument\n",
				wantCode: 2,
			},
			{
				name:     "non-integer operand on the left",
				args:     []string{"a", "+", "1"},
				wantErr:  "expr: non-integer argument\n",
				wantCode: 2,
			},
			{
				name:     "operand beyond the 64-bit range",
				args:     []string{"1", "+", "99999999999999999999"},
				wantErr:  "expr: 99999999999999999999: Numerical result out of range\n",
				wantCode: 3,
			},
			{
				name:     "negative operand beyond the 64-bit range",
				args:     []string{"-99999999999999999999", "+", "1"},
				wantErr:  "expr: -99999999999999999999: Numerical result out of range\n",
				wantCode: 3,
			},
			{
				name:     "sum beyond the 64-bit range",
				args:     []string{"9223372036854775807", "+", "1"},
				wantErr:  "expr: Numerical result out of range\n",
				wantCode: 3,
			},
			{
				name:     "difference beyond the 64-bit range",
				args:     []string{"-9223372036854775808", "-", "1"},
				wantErr:  "expr: Numerical result out of range\n",
				wantCode: 3,
			},
			{
				name:     "product beyond the 64-bit range",
				args:     []string{"9223372036854775807", "*", "2"},
				wantErr:  "expr: Numerical result out of range\n",
				wantCode: 3,
			},
			{
				name:     "negating the lowest integer overflows",
				args:     []string{"-9223372036854775808", "*", "-1"},
				wantErr:  "expr: Numerical result out of range\n",
				wantCode: 3,
			},
			{
				// The signs multiply to a positive result, so the
				// reversed order overflows just the same.
				name:     "minus one times the lowest integer overflows",
				args:     []string{"-1", "*", "-9223372036854775808"},
				wantErr:  "expr: Numerical result out of range\n",
				wantCode: 3,
			},
			{
				name:     "product of the lowest integer by two overflows",
				args:     []string{"-9223372036854775808", "*", "2"},
				wantErr:  "expr: Numerical result out of range\n",
				wantCode: 3,
			},
			{
				name:     "division of the lowest integer by minus one",
				args:     []string{"-9223372036854775808", "/", "-1"},
				wantErr:  "expr: Numerical result out of range\n",
				wantCode: 3,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				out, stderr, err := runExpr(t, test.args...)
				if code := testutil.ExitCode(t, err); code != test.wantCode {
					t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
				}
				if out != "" {
					t.Errorf("Run() stdout = %q, want empty", out)
				}
				if stderr != test.wantErr {
					t.Errorf("Run() stderr = %q, want %q", stderr, test.wantErr)
				}
			})
		}
	})
}

func TestComparison(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
	}{
		{
			name:    "less than",
			args:    []string{"1", "<", "2"},
			wantOut: "1\n",
		},
		{
			name:     "less than is false",
			args:     []string{"2", "<", "1"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "less than or equal",
			args:    []string{"2", "<=", "2"},
			wantOut: "1\n",
		},
		{
			name:    "equal",
			args:    []string{"3", "=", "3"},
			wantOut: "1\n",
		},
		{
			name:     "equal is false",
			args:     []string{"3", "=", "4"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "double equals is a synonym of equal",
			args:    []string{"3", "==", "3"},
			wantOut: "1\n",
		},
		{
			name:    "not equal",
			args:    []string{"3", "!=", "4"},
			wantOut: "1\n",
		},
		{
			name:     "not equal is false",
			args:     []string{"3", "!=", "3"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "greater than",
			args:    []string{"5", ">", "4"},
			wantOut: "1\n",
		},
		{
			name:     "greater than is false",
			args:     []string{"4", ">", "5"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "greater than or equal",
			args:    []string{"5", ">=", "5"},
			wantOut: "1\n",
		},
		{
			name:     "compares integers numerically, not bytewise",
			args:     []string{"10", "<", "9"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "compares strings bytewise",
			args:    []string{"abc", "<", "abd"},
			wantOut: "1\n",
		},
		{
			name:    "compares mixed operands as strings",
			args:    []string{"10", "<", "9x"},
			wantOut: "1\n",
		},
		{
			name:    "compares negative integers numerically",
			args:    []string{"-5", "<", "5"},
			wantOut: "1\n",
		},
		{
			name:    "equates minus zero with zero",
			args:    []string{"-0", "=", "0"},
			wantOut: "1\n",
		},
		{
			name:    "equates leading zeros",
			args:    []string{"007", "=", "7"},
			wantOut: "1\n",
		},
		{
			name:    "compares integers beyond the 64-bit range",
			args:    []string{"99999999999999999999", ">", "99999999999999999998"},
			wantOut: "1\n",
		},
		{
			name:    "equates huge integers",
			args:    []string{"99999999999999999999", "=", "99999999999999999999"},
			wantOut: "1\n",
		},
		{
			name:    "chains comparisons to the left",
			args:    []string{"1", "<", "2", "<", "3"},
			wantOut: "1\n",
		},
		{
			name:    "compares with a computed operand",
			args:    []string{"1", "+", "1", "=", "2"},
			wantOut: "1\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, _, err := runExpr(t, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if out != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", out, test.wantOut)
			}
		})
	}
}

func TestLogical(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
	}{
		{
			name:    "or keeps a non-null first operand",
			args:    []string{"1", "|", "2"},
			wantOut: "1\n",
		},
		{
			name:    "or keeps a non-null string first operand",
			args:    []string{"abc", "|", "def"},
			wantOut: "abc\n",
		},
		{
			name:    "or falls back to the second operand",
			args:    []string{"0", "|", "2"},
			wantOut: "2\n",
		},
		{
			name:    "or falls back to the second string operand",
			args:    []string{"", "|", "def"},
			wantOut: "def\n",
		},
		{
			name:     "or of two null operands gives zero",
			args:     []string{"", "|", ""},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "or keeps the value of the first operand",
			args:    []string{"5", "|", "6"},
			wantOut: "5\n",
		},
		{
			name:    "and keeps the first operand when neither is null",
			args:    []string{"1", "&", "2"},
			wantOut: "1\n",
		},
		{
			name:    "and keeps a string first operand",
			args:    []string{"abc", "&", "def"},
			wantOut: "abc\n",
		},
		{
			name:     "and gives zero when the second operand is null",
			args:     []string{"1", "&", "0"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:     "and gives zero when the first operand is null",
			args:     []string{"0", "&", "2"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:     "and gives zero for a null string",
			args:     []string{"", "&", "x"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:     "or has the lowest precedence",
			args:     []string{"0", "|", "1", "&", "0"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "or is left-associative",
			args:    []string{"0", "|", "0", "|", "3"},
			wantOut: "3\n",
		},
		{
			name:     "and holds quoted tokens as strings",
			args:     []string{"+", "0", "&", "1"},
			wantOut:  "0\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, _, err := runExpr(t, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if out != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", out, test.wantOut)
			}
		})
	}

	t.Run("short-circuits", func(t *testing.T) {
		tests := []struct {
			name     string
			args     []string
			wantOut  string
			wantCode int
		}{
			{
				// The second operand of | is not evaluated, so the
				// division by zero never happens.
				name:    "or skips the second operand",
				args:    []string{"1", "|", "1", "/", "0"},
				wantOut: "1\n",
			},
			{
				// The second operand of & is not evaluated when the
				// first one is null.
				name:     "and skips the second operand",
				args:     []string{"0", "&", "1", "/", "0"},
				wantOut:  "0\n",
				wantCode: 1,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				out, stderr, err := runExpr(t, test.args...)
				if code := testutil.ExitCode(t, err); code != test.wantCode {
					t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
				}
				if stderr != "" {
					t.Errorf("Run() stderr = %q, want empty", stderr)
				}
				if out != test.wantOut {
					t.Errorf("Run() stdout = %q, want %q", out, test.wantOut)
				}
			})
		}
	})
}

func TestStringKeywords(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
	}{
		{
			name:    "length counts the characters",
			args:    []string{"length", "abcdef"},
			wantOut: "6\n",
		},
		{
			name:     "length of the null string",
			args:     []string{"length", ""},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "length of a parenthesized operand",
			args:    []string{"length", "(", "abc", ")"},
			wantOut: "3\n",
		},
		{
			name:    "length of a computed operand, as its string form",
			args:    []string{"length", "(", "1", "+", "1", ")"},
			wantOut: "1\n",
		},
		{
			name:    "index finds the first character of the set",
			args:    []string{"index", "abcdef", "cz"},
			wantOut: "3\n",
		},
		{
			name:    "index finds the first character of the set, not of the string",
			args:    []string{"index", "abcdef", "dx"},
			wantOut: "4\n",
		},
		{
			name:     "index without a match gives zero",
			args:     []string{"index", "abcdef", "xyz"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:     "index with an empty set gives zero",
			args:     []string{"index", "abcdef", ""},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "substr takes characters from the position on",
			args:    []string{"substr", "abcdef", "3", "2"},
			wantOut: "cd\n",
		},
		{
			name:    "substr stops at the end of the string",
			args:    []string{"substr", "abcdef", "2", "100"},
			wantOut: "bcdef\n",
		},
		{
			name:     "substr of a zero position",
			args:     []string{"substr", "abcdef", "0", "5"},
			wantOut:  "\n",
			wantCode: 1,
		},
		{
			name:     "substr of a negative position",
			args:     []string{"substr", "abcdef", "-1", "2"},
			wantOut:  "\n",
			wantCode: 1,
		},
		{
			name:     "substr of a zero length",
			args:     []string{"substr", "abcdef", "2", "0"},
			wantOut:  "\n",
			wantCode: 1,
		},
		{
			name:     "substr of a negative length",
			args:     []string{"substr", "abcdef", "2", "-1"},
			wantOut:  "\n",
			wantCode: 1,
		},
		{
			name:     "substr past the end of the string",
			args:     []string{"substr", "abcdef", "10", "5"},
			wantOut:  "\n",
			wantCode: 1,
		},
		{
			name:     "substr of a non-numeric position",
			args:     []string{"substr", "abc", "a", "2"},
			wantOut:  "\n",
			wantCode: 1,
		},
		{
			name:     "substr of a non-numeric length",
			args:     []string{"substr", "abc", "1", "x"},
			wantOut:  "\n",
			wantCode: 1,
		},
		{
			name:     "substr of a huge position gives the null string",
			args:     []string{"substr", "abcdef", "99999999999999999999", "1"},
			wantOut:  "\n",
			wantCode: 1,
		},
		{
			name:     "substr of a huge negative position gives the null string",
			args:     []string{"substr", "abcdef", "-99999999999999999999", "1"},
			wantOut:  "\n",
			wantCode: 1,
		},
		{
			name:    "substr of a huge length gives the rest of the string",
			args:    []string{"substr", "abcdef", "2", "99999999999999999999"},
			wantOut: "bcdef\n",
		},
		{
			name:    "match is the : operator",
			args:    []string{"match", "abcdef", "abc"},
			wantOut: "3\n",
		},
		{
			name:    "match with a group returns the group",
			args:    []string{"match", "abcdef", `a\(..\)d`},
			wantOut: "bc\n",
		},
		{
			name:     "match without a match and a group gives the null string",
			args:     []string{"match", "abcdef", `x\(y\)`},
			wantOut:  "\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, _, err := runExpr(t, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if out != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", out, test.wantOut)
			}
		})
	}

	t.Run("multibyte", func(t *testing.T) {
		tests := []struct {
			name     string
			args     []string
			wantOut  string
			wantCode int
		}{
			{
				name:    "length counts characters, not bytes",
				args:    []string{"length", "héllo"},
				wantOut: "5\n",
			},
			{
				name:    "substr takes characters, not bytes",
				args:    []string{"substr", "héllo", "2", "2"},
				wantOut: "él\n",
			},
			{
				name:    "index counts characters, not bytes",
				args:    []string{"index", "héllo", "o"},
				wantOut: "5\n",
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				out, _, err := runExpr(t, test.args...)
				if code := testutil.ExitCode(t, err); code != test.wantCode {
					t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
				}
				if out != test.wantOut {
					t.Errorf("Run() stdout = %q, want %q", out, test.wantOut)
				}
			})
		}
	})
}

func TestPatternMatching(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
	}{
		{
			name:    "counts the characters matched",
			args:    []string{"abc", ":", "a"},
			wantOut: "1\n",
		},
		{
			name:     "counts zero characters on no match",
			args:     []string{"abc", ":", "b"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "counts the whole string with a dot star",
			args:    []string{"abc", ":", ".*"},
			wantOut: "3\n",
		},
		{
			name:     "matches an empty pattern",
			args:     []string{"abc", ":", ""},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "anchors the pattern at the beginning",
			args:    []string{"abc", ":", "^a"},
			wantOut: "1\n",
		},
		{
			name:     "rejects a leading caret anchor on a later character",
			args:     []string{"abc", ":", "^b"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "returns the first group when the pattern uses one",
			args:    []string{"abcdef", ":", `a\(bc\)d`},
			wantOut: "bc\n",
		},
		{
			name:    "returns the first group when the pattern uses more",
			args:    []string{"abcdef", ":", `a\(bc\)d\(ef\)`},
			wantOut: "bc\n",
		},
		{
			name:    "returns the whole string with a dot star group",
			args:    []string{"abcdef", ":", `\(.*\)`},
			wantOut: "abcdef\n",
		},
		{
			name:    "returns the matched part, not the whole pattern",
			args:    []string{"abcdef", ":", `.\(.\)`},
			wantOut: "b\n",
		},
		{
			name:     "gives the null string when the group did not match",
			args:     []string{"abcdef", ":", `\(x\)`},
			wantOut:  "\n",
			wantCode: 1,
		},
		{
			name:     "gives the null string when the group did not contribute",
			args:     []string{"a", ":", `a\(x\)*`},
			wantOut:  "\n",
			wantCode: 1,
		},
		{
			name:     "matches an empty string against an empty pattern",
			args:     []string{"", ":", ""},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "matches a bracket expression at the beginning",
			args:    []string{"hello", ":", `[hel][hel]*`},
			wantOut: "4\n",
		},
		{
			name:     "a plus after a bracket is an ordinary character",
			args:     []string{"hello", ":", `[hel]+`},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "counts characters, not bytes, of the match",
			args:    []string{"héllo", ":", `h.`},
			wantOut: "2\n",
		},
		{
			name:    "returns a group with multibyte characters",
			args:    []string{"héllo", ":", `h\(.\)l`},
			wantOut: "é\n",
		},
		{
			// The GNU \+ quantifier is unsupported by the basic regular
			// expressions of internal/bre; see the deviations section of
			// coreutils/README.md.
			name:     "the GNU plus quantifier is ordinary in a pattern",
			args:     []string{"aaa", ":", "a\\+"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "the match result feeds an arithmetic operation",
			args:    []string{"abc", ":", ".*", "+", "1"},
			wantOut: "4\n",
		},
		{
			name:    "gives a matched count with a false relation",
			args:    []string{"abc", ":", ".*", "<", "10"},
			wantOut: "1\n",
		},
		{
			name:    "or falls back after a failed match with a group",
			args:    []string{"abc", ":", `\(x\)`, "|", "fallback"},
			wantOut: "fallback\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, _, err := runExpr(t, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if out != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", out, test.wantOut)
			}
		})
	}

	t.Run("invalid patterns", func(t *testing.T) {
		tests := []struct {
			name     string
			args     []string
			wantCode int
		}{
			{
				name:     "unmatched \\(",
				args:     []string{"abc", ":", `a\(`},
				wantCode: 2,
			},
			{
				name:     "unmatched \\)",
				args:     []string{"abc", ":", `a\)`},
				wantCode: 2,
			},
			{
				name:     "backreferences are not supported",
				args:     []string{"abc", ":", `\(a\)\1`},
				wantCode: 2,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				out, stderr, err := runExpr(t, test.args...)
				if code := testutil.ExitCode(t, err); code != test.wantCode {
					t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
				}
				if out != "" {
					t.Errorf("Run() stdout = %q, want empty", out)
				}
				if !bytes.HasPrefix([]byte(stderr), []byte("expr: ")) {
					t.Errorf("Run() stderr = %q, want a diagnostic", stderr)
				}
			})
		}
	})
}

func TestQuotingAndGrouping(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
	}{
		{
			name:    "plus quotes the next token as a string",
			args:    []string{"+", "match"},
			wantOut: "match\n",
		},
		{
			name:    "plus quotes an operator as a string",
			args:    []string{"+", "/"},
			wantOut: "/\n",
		},
		{
			name:    "plus quotes a negative integer",
			args:    []string{"+", "-5"},
			wantOut: "-5\n",
		},
		{
			name:    "plus lets a keyword apply to a keyword-like string",
			args:    []string{"length", "+", "abc"},
			wantOut: "3\n",
		},
		{
			name:     "plus quotes the index operand of the manual example",
			args:     []string{"index", "+", "index", "a"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "plus quotes an operand of a comparison",
			args:    []string{"+", "5", "=", "5"},
			wantOut: "1\n",
		},
		{
			name:    "the addition operator still applies after a quoted token",
			args:    []string{"+", "5", "+", "3"},
			wantOut: "8\n",
		},
		{
			name:    "the quoted token takes the form of an integer",
			args:    []string{"+", "1", "|", "2"},
			wantOut: "1\n",
		},
		{
			// The GNU manual's expr + "$x" : '.*/\(.\)' shape: the
			// quote token starts the left operand of :.
			name:    "plus starts the string of a match",
			args:    []string{"+", "abc", ":", `a\(.\)`},
			wantOut: "b\n",
		},
		{
			// Each + at an operand position quotes the token after it;
			// only the ones between operands add.
			name:    "plus quotes every operand of a sum",
			args:    []string{"+", "5", "+", "2", "+", "3"},
			wantOut: "10\n",
		},
		{
			// The GNU manual's expr length + "$x" shape, on the right
			// of a comparison: a quoted keyword is a plain string.
			name:     "plus quotes a keyword on the right of a comparison",
			args:     []string{"abc", "=", "+", "length"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "plus starts the pattern of a match keyword",
			args:    []string{"match", "x", "+", "x"},
			wantOut: "1\n",
		},
		{
			name:    "plus starts a parenthesized subexpression",
			args:    []string{"(", "+", "5", ")", "+", "1"},
			wantOut: "6\n",
		},
		{
			name:    "parentheses group an expression",
			args:    []string{"(", "1", "+", "2", ")", "*", "3"},
			wantOut: "9\n",
		},
		{
			name:    "parentheses nest",
			args:    []string{"(", "(", "1", ")", ")"},
			wantOut: "1\n",
		},
		{
			name:    "a whole token is a string, however it looks",
			args:    []string{"1|2"},
			wantOut: "1|2\n",
		},
		{
			name:    "plus quotes a parenthesis",
			args:    []string{"+", "("},
			wantOut: "(\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, _, err := runExpr(t, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if out != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", out, test.wantOut)
			}
		})
	}
}

func TestNullAndStatus(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
	}{
		{
			name:     "zero is printed and exits with 1",
			args:     []string{"0"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:     "a null string is printed as an empty line and exits with 1",
			args:     []string{""},
			wantOut:  "\n",
			wantCode: 1,
		},
		{
			name:     "zeroes with leading zeros are null",
			args:     []string{"00"},
			wantOut:  "00\n",
			wantCode: 1,
		},
		{
			name:     "minus zero is null",
			args:     []string{"-0"},
			wantOut:  "-0\n",
			wantCode: 1,
		},
		{
			name:     "a null string is null",
			args:     []string{"00", "|", ""},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "a minus alone is a string, not null",
			args:    []string{"-"},
			wantOut: "-\n",
		},
		{
			name:     "an arithmetic result of zero exits with 1",
			args:     []string{"0", "+", "0"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "a non-null string exits with 0",
			args:    []string{"abc"},
			wantOut: "abc\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, _, err := runExpr(t, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if out != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", out, test.wantOut)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantErr  string
		wantCode int
	}{
		{
			name:     "missing operand",
			args:     nil,
			wantErr:  "expr: missing operand\nTry 'expr --help' for more information.\n",
			wantCode: 2,
		},
		{
			name:     "unexpected argument",
			args:     []string{"1", "2"},
			wantErr:  "expr: syntax error: unexpected argument '2'\n",
			wantCode: 2,
		},
		{
			name:     "missing argument after an operator",
			args:     []string{"1", "+"},
			wantErr:  "expr: syntax error: missing argument after '+'\n",
			wantCode: 2,
		},
		{
			name:     "missing argument after a keyword",
			args:     []string{"length"},
			wantErr:  "expr: syntax error: missing argument after 'length'\n",
			wantCode: 2,
		},
		{
			name:     "missing argument after a quote token",
			args:     []string{"+"},
			wantErr:  "expr: syntax error: missing argument after '+'\n",
			wantCode: 2,
		},
		{
			name:     "missing argument after a parenthesis",
			args:     []string{"("},
			wantErr:  "expr: syntax error: missing argument after '('\n",
			wantCode: 2,
		},
		{
			name:     "missing closing parenthesis",
			args:     []string{"(", "1", "+", "2"},
			wantErr:  "expr: syntax error: expecting ')' after '2'\n",
			wantCode: 2,
		},
		{
			name:     "wrong closing parenthesis",
			args:     []string{"(", "1", "+", "2", "x"},
			wantErr:  "expr: syntax error: expecting ')' instead of 'x'\n",
			wantCode: 2,
		},
		{
			name:     "closing parenthesis without an opening one",
			args:     []string{"(", ")"},
			wantErr:  "expr: syntax error: unexpected ')'\n",
			wantCode: 2,
		},
		{
			name:     "quotes the argument of a diagnostic",
			args:     []string{"1", "it's"},
			wantErr:  `expr: syntax error: unexpected argument 'it'\''s'` + "\n",
			wantCode: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, stderr, err := runExpr(t, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if out != "" {
				t.Errorf("Run() stdout = %q, want empty", out)
			}
			if stderr != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", stderr, test.wantErr)
			}
		})
	}
}

func TestEndOfOptions(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
	}{
		{
			name:    "a leading -- is skipped",
			args:    []string{"--", "1", "+", "1"},
			wantOut: "2\n",
		},
		{
			name:     "a leading -- protects a negative integer",
			args:     []string{"--", "-1", "+", "1"},
			wantOut:  "0\n",
			wantCode: 1,
		},
		{
			name:    "a leading -- protects a leading minus string",
			args:    []string{"--", "-abc"},
			wantOut: "-abc\n",
		},
		{
			name:    "only one -- is skipped",
			args:    []string{"--", "--"},
			wantOut: "--\n",
		},
		{
			name:     "a negative integer is an operand, not an option",
			args:     []string{"-1", "+", "1"},
			wantOut:  "0\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, _, err := runExpr(t, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if out != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", out, test.wantOut)
			}
		})
	}
}

func TestPOSIXExamples(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
	}{
		{
			name:    "the increment idiom",
			args:    []string{"10", "+", "1"},
			wantOut: "11\n",
		},
		{
			name:    "the non-directory part of a file name",
			args:    []string{"dir/name", ":", `.*/\(.*\)`, "|", "dir/name"},
			wantOut: "name\n",
		},
		{
			name:    "the non-directory part of a plain file name",
			args:    []string{"name", ":", `.*/\(.*\)`, "|", "name"},
			wantOut: "name\n",
		},
		{
			name:    "an anchor protects from a leading operator",
			args:    []string{"Xabc", "=", "Xabc"},
			wantOut: "1\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, _, err := runExpr(t, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if out != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", out, test.wantOut)
			}
		})
	}
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "expr", usage)
}

func TestStopOnCanceledContext(t *testing.T) {
	testutil.Cancelled(t, New, nil, nil, "1", "+", "1")
}

func TestWriteError(t *testing.T) {
	writeErr := errors.New("write error")
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: testutil.ErrorWriter{Err: writeErr},
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context(), "1", "+", "1")
	if !errors.Is(err, writeErr) {
		t.Errorf("Run() error = %v, want it to wrap the write error", err)
	}
	// GNU exits with EXPR_FAILURE on a failed write, through close_stdout.
	if code := testutil.ExitCode(t, err); code != 3 {
		t.Errorf("Run() exit code = %d, want 3", code)
	}
	if got, want := stderr.String(), "expr: write error: write error\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}

// TestStrintcmp covers the comparison of integer strings beyond the 64-bit
// range, including the leading zeros and the signs.
func TestStrintcmp(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0", "0", 0},
		{"0", "1", -1},
		{"9", "10", -1},
		{"10", "9", 1},
		{"100", "99", 1},
		{"123", "123", 0},
		{"007", "7", 0},
		{"-0", "0", 0},
		{"-0", "-0", 0},
		{"-1", "1", -1},
		{"1", "-1", 1},
		{"-1", "-2", 1},
		{"-2", "-1", -1},
		{"-10", "-9", -1},
		{"99999999999999999998", "99999999999999999999", -1},
		{"99999999999999999999", "100000000000000000000", -1},
		{"-99999999999999999999", "-100000000000000000000", 1},
	}
	for _, test := range tests {
		if got := strintcmp(test.a, test.b); (got < 0) != (test.want < 0) || (got == 0) != (test.want == 0) {
			t.Errorf("strintcmp(%q, %q) = %d, want %d", test.a, test.b, got, test.want)
		}
	}
}
