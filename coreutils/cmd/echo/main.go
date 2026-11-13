// Command echo is a thin wrapper around the echo core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/echo"
)

func main() {
	cmdutil.Main(echo.New)
}
