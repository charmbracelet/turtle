// Command base32 is a thin wrapper around the base32 core utility.
package main

import (
	"charm.land/snail/coreutils/base32"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(base32.New)
}
