// Command expr is a thin wrapper around the expr core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/expr"
)

func main() {
	cmdutil.Main(expr.New)
}
