// Command dd is a thin wrapper around the dd core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/dd"
)

func main() {
	cmdutil.Main(dd.New)
}
