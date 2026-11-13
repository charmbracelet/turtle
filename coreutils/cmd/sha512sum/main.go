// Command sha512sum is a thin wrapper around the sha512sum core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/sha512sum"
)

func main() {
	cmdutil.Main(sha512sum.New)
}
