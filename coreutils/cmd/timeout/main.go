// Command timeout is a thin wrapper around the timeout core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/timeout"
)

func main() {
	cmdutil.Main(timeout.New)
}
