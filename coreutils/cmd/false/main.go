// Command false is a thin wrapper around the false core utility.
package main

import (
	"charm.land/turtle/coreutils/false"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(false.New)
}
