// Command nice is a thin wrapper around the nice core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/nice"
)

func main() {
	cmdutil.Main(nice.New)
}
