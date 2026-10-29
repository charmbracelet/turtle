// Command env is a thin wrapper around the env core utility.
package main

import (
	"charm.land/snail/coreutils/env"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(env.New)
}
