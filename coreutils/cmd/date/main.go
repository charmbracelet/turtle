// Command date is a thin wrapper around the date core utility.
package main

import (
	"charm.land/snail/coreutils/date"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(date.New)
}
