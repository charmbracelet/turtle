// Command mktemp is a thin wrapper around the mktemp core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/mktemp"
)

func main() {
	cmdutil.Main(mktemp.New)
}
