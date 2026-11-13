// Command base32 is a thin wrapper around the base32 core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/base32"
)

func main() {
	cmdutil.Main(base32.New)
}
