// Command tee is a thin wrapper around the tee core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/tee"
)

func main() {
	cmdutil.Main(tee.New)
}
