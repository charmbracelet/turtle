//go:build linux

package uname

import "syscall"

// utsFields gathers the system information of the host from uname(2), with
// the operating system name given, since it differs between the Linux
// flavors.
func utsFields(osName string) (fields, error) {
	var uts syscall.Utsname
	if err := syscall.Uname(&uts); err != nil {
		return fields{}, err
	}
	machine := utsString(uts.Machine[:])
	return fields{
		sysname:  utsString(uts.Sysname[:]),
		nodename: utsString(uts.Nodename[:]),
		release:  utsString(uts.Release[:]),
		version:  utsString(uts.Version[:]),
		machine:  machine,
		// The hardware platform repeats the machine value, as the GNU
		// utility reports on Linux, and the processor type stays unknown,
		// as it does there too.
		hardwarePlatform: machine,
		osName:           osName,
	}, nil
}

// utsString reads a NUL-terminated buffer of a utsname field.
func utsString[N ~int8 | ~uint8](b []N) string {
	var out []byte
	for _, c := range b {
		if c == 0 {
			break
		}
		out = append(out, byte(c))
	}
	return string(out)
}
