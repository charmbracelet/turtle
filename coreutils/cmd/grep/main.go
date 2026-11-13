// Command grep is a thin wrapper around the grep core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/grep"
)

func main() {
	cmdutil.Main(grep.New)
}
