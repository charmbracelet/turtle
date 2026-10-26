// Command cksum is a thin wrapper around the cksum core utility.
package main

import (
	"charm.land/snail/coreutils/cksum"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(cksum.New)
}
