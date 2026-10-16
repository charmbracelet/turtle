// Command pwd is a thin wrapper around the pwd core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/pwd"
)

func main() {
	cmdutil.Main(pwd.New)
}
