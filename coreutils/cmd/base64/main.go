// Command base64 is a thin wrapper around the base64 core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/base64"
)

func main() {
	cmdutil.Main(base64.New)
}
