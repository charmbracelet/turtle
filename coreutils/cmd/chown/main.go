// Command chown is a thin wrapper around the chown core utility.
package main

import (
	"charm.land/snail/coreutils/chown"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(chown.New)
}
