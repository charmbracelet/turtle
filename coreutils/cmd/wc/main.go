// Command wc is a thin wrapper around the wc core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/wc"
)

func main() {
	cmdutil.Main(wc.New)
}
