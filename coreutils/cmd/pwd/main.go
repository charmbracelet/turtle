// Command pwd is a thin wrapper around the pwd core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/pwd"
)

func main() {
	cmdutil.Main(pwd.New)
}
