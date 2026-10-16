// Command wc is a thin wrapper around the wc core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/wc"
)

func main() {
	cmdutil.Main(wc.New)
}
