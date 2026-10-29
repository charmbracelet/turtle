// Command test is a thin wrapper around the test core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/test"
)

func main() {
	cmdutil.Main(test.New)
}
