//go:build linux && !android

package uname

// hostFields gathers the system information of the host from uname(2).
func hostFields() (fields, error) {
	return utsFields("GNU/Linux")
}
