// Command sha384sum is a thin wrapper around the sha384sum core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/sha384sum"
)

func main() {
	cmdutil.Main(sha384sum.New)
}
