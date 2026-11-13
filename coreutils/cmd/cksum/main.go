// Command cksum is a thin wrapper around the cksum core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/cksum"
)

func main() {
	cmdutil.Main(cksum.New)
}
