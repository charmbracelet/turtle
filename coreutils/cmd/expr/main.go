// Command expr is a thin wrapper around the expr core utility.
package main

import (
	"charm.land/turtle/coreutils/expr"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(expr.New)
}
