// Command mktemp is a thin wrapper around the mktemp core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/mktemp"
)

func main() {
	cmdutil.Main(mktemp.New)
}
