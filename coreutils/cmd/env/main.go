// Command env is a thin wrapper around the env core utility.
package main

import (
	"charm.land/turtle/coreutils/env"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(env.New)
}
