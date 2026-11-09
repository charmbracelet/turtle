// Command dd is a thin wrapper around the dd core utility.
package main

import (
	"charm.land/snail/coreutils/dd"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(dd.New)
}
