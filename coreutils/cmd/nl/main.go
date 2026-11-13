// nl is a thin wrapper around the nl core utility implementation.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/nl"
)

func main() {
	cmdutil.Main(nl.New)
}
