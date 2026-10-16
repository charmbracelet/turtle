// Command base64 is a thin wrapper around the base64 core utility.
package main

import (
	"charm.land/snail/coreutils/base64"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(base64.New)
}
