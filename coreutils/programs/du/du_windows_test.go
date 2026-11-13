//go:build windows

package du

import (
	"testing"

	"charm.land/turtle/coreutils/internal/testutil"
)

// On Windows the operands may name their files with backslashes, which are
// normalized like the GNU utility does with its ISSLASH macro.
func TestRunBackslashOperands(t *testing.T) {
	testutil.Once(t, New, testutil.Case{
		Name: "a backslash operand names its file",
		FS:   tree,
		Args: []string{"tree\\a"},
		Want: "2 tree/a\n",
	})
}
