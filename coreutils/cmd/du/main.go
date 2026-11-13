// Command du is a thin wrapper around the du core utility.
package main

import (
	"charm.land/turtle/coreutils/du"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(du.New)
}
