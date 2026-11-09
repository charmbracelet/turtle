// Command chgrp is a thin wrapper around the chgrp core utility.
package main

import (
	"charm.land/snail/coreutils/chgrp"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(chgrp.New)
}
