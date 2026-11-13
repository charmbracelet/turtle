// Command cp is a thin wrapper around the cp core utility.
package main

import (
	"charm.land/turtle/coreutils/cp"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(cp.New)
}
