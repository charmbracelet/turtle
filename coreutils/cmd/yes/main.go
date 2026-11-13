// Command yes is a thin wrapper around the yes core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/yes"
)

func main() {
	cmdutil.Main(yes.New)
}
