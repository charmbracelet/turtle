// Command grep is a thin wrapper around the grep core utility.
package main

import (
	"charm.land/turtle/coreutils/grep"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(grep.New)
}
