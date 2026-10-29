// Command whoami is a thin wrapper around the whoami core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/whoami"
)

func main() {
	cmdutil.Main(whoami.New)
}
