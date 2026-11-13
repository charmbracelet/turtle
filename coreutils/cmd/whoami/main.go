// Command whoami is a thin wrapper around the whoami core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/whoami"
)

func main() {
	cmdutil.Main(whoami.New)
}
