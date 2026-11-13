// Command date is a thin wrapper around the date core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/date"
)

func main() {
	cmdutil.Main(date.New)
}
