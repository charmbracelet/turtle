//go:build !unix && !windows

package mv

import "errors"

// exdev stands in for the error a rename across file systems fails with. No
// platform here reports one, so it only ever matches errors built from this
// very value, like the ones tests use.
var exdev = errors.New("invalid cross-device link")
