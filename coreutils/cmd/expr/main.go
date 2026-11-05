// Command expr is a thin wrapper around the expr core utility.
package main

import (
	"charm.land/snail/coreutils/expr"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(expr.New)
}
