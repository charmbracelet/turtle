// Command cp is a thin wrapper around the cp core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/cp"
)

func main() {
	cmdutil.Main(cp.New)
}
