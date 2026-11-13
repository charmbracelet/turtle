// Command install is a thin wrapper around the install core utility.
package main

import (
	"charm.land/turtle/coreutils/install"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(install.New)
}
