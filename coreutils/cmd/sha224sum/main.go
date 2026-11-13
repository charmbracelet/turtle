// Command sha224sum is a thin wrapper around the sha224sum core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/sha224sum"
)

func main() {
	cmdutil.Main(sha224sum.New)
}
