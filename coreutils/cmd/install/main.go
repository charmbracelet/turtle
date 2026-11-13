// Command install is a thin wrapper around the install core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/install"
)

func main() {
	cmdutil.Main(install.New)
}
