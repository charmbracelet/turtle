// Command grep is a thin wrapper around the grep core utility.
package main

import (
	"charm.land/snail/coreutils/grep"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(grep.New)
}
