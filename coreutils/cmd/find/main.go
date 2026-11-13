// Command find is a thin wrapper around the find core utility.
package main

import (
	"charm.land/turtle/coreutils/find"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(find.New)
}
