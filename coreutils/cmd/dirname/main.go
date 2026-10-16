// Command dirname is a thin wrapper around the dirname core utility.
package main

import (
	"charm.land/snail/coreutils/dirname"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(dirname.New)
}
