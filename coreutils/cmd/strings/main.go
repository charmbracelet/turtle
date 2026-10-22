// Command strings is a thin wrapper around the strings core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/strings"
)

func main() {
	cmdutil.Main(strings.New)
}
