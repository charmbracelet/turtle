//go:build !(aix || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris)

package fsx

// sysOwner reports no owner on systems that don't model file ownership.
func sysOwner(any) (int, int, bool) {
	return 0, 0, false
}
