// Command strings is a thin wrapper around the strings core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/strings"
)

func main() {
	cmdutil.Main(strings.New)
}
