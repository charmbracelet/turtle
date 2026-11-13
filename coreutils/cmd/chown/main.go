// Command chown is a thin wrapper around the chown core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/chown"
)

func main() {
	cmdutil.Main(chown.New)
}
