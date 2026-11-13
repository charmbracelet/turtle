// Command dd is a thin wrapper around the dd core utility.
package main

import (
	"charm.land/turtle/coreutils/dd"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(dd.New)
}
