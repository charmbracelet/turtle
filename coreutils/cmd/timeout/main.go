// Command timeout is a thin wrapper around the timeout core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/timeout"
)

func main() {
	cmdutil.Main(timeout.New)
}
