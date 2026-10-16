// Command true is a thin wrapper around the true core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/true"
)

func main() {
	cmdutil.Main(true.New)
}
