// Command printf is a thin wrapper around the printf core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/printf"
)

func main() {
	cmdutil.Main(printf.New)
}
