// Command du is a thin wrapper around the du core utility.
package main

import (
	"charm.land/snail/coreutils/du"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(du.New)
}
