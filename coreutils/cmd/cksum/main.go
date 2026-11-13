// Command cksum is a thin wrapper around the cksum core utility.
package main

import (
	"charm.land/turtle/coreutils/cksum"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(cksum.New)
}
