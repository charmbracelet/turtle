// Command uuencode is a thin wrapper around the uuencode core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/uuencode"
)

func main() {
	cmdutil.Main(uuencode.New)
}
