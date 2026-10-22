// nl is a thin wrapper around the nl core utility implementation.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/nl"
)

func main() {
	cmdutil.Main(nl.New)
}
