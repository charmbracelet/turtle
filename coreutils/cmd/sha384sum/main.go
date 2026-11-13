// Command sha384sum is a thin wrapper around the sha384sum core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/sha384sum"
)

func main() {
	cmdutil.Main(sha384sum.New)
}
