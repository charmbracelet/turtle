// Command paste is a thin wrapper around the paste core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/paste"
)

func main() {
	cmdutil.Main(paste.New)
}
