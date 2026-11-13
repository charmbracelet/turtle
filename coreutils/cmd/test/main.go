// Command test is a thin wrapper around the test core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/test"
)

func main() {
	cmdutil.Main(test.New)
}
