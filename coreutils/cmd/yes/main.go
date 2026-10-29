// Command yes is a thin wrapper around the yes core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/yes"
)

func main() {
	cmdutil.Main(yes.New)
}
