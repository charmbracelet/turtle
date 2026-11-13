// Command printf is a thin wrapper around the printf core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/printf"
)

func main() {
	cmdutil.Main(printf.New)
}
