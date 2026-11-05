//go:build android

package uname

// hostFields gathers the system information of the host from uname(2). The
// kernel reports Linux, but the operating system is Android, as the GNU
// port of uname reports on Android systems.
func hostFields() (fields, error) {
	return utsFields("Android")
}
