// Command base32 is a thin wrapper around the base32 core utility.
package main

import (
	"charm.land/turtle/coreutils/base32"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(base32.New)
}
