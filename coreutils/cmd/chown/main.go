// Command chown is a thin wrapper around the chown core utility.
package main

import (
	"charm.land/turtle/coreutils/chown"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(chown.New)
}
