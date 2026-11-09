//go:build windows

package du

import "testing"

// On Windows the operands may name their files with backslashes, which are
// normalized like the GNU utility does with its ISSLASH macro.
func TestRunBackslashOperands(t *testing.T) {
	runCase(t, testCase{
		name:    "a backslash operand names its file",
		files:   tree,
		args:    []string{"tree\\a"},
		wantOut: "2 tree/a\n",
	})
}
