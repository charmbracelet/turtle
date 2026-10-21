// Command uuencode is a thin wrapper around the uuencode core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/uuencode"
)

func main() {
	cmdutil.Main(uuencode.New)
}
