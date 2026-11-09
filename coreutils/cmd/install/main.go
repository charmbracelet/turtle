// Command install is a thin wrapper around the install core utility.
package main

import (
	"charm.land/snail/coreutils/install"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(install.New)
}
