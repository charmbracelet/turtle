// Command shasum is a thin wrapper around the shasum core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/shasum"
)

func main() {
	cmdutil.Main(shasum.New)
}
