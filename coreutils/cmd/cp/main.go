// Command cp is a thin wrapper around the cp core utility.
package main

import (
	"charm.land/snail/coreutils/cp"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(cp.New)
}
