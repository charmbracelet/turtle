// Command echo is a thin wrapper around the echo core utility.
package main

import (
	"charm.land/snail/coreutils/echo"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(echo.New)
}
