// Command date is a thin wrapper around the date core utility.
package main

import (
	"charm.land/turtle/coreutils/date"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(date.New)
}
