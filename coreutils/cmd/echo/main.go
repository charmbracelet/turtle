// Command echo is a thin wrapper around the echo core utility.
package main

import (
	"charm.land/turtle/coreutils/echo"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(echo.New)
}
