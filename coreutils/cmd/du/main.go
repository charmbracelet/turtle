// Command du is a thin wrapper around the du core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/du"
)

func main() {
	cmdutil.Main(du.New)
}
