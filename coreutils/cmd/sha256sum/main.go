// Command sha256sum is a thin wrapper around the sha256sum core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/sha256sum"
)

func main() {
	cmdutil.Main(sha256sum.New)
}
