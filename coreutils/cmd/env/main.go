// Command env is a thin wrapper around the env core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/env"
)

func main() {
	cmdutil.Main(env.New)
}
