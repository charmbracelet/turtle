// Command false is a thin wrapper around the false core utility.
package main

import (
	"charm.land/snail/coreutils/false"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(false.New)
}
