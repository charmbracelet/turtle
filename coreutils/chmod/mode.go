package chmod

import (
	"io/fs"

	"charm.land/snail/coreutils/internal/filemode"
)

// ModeSpec computes the new mode of a file from its current mode, whether
// it's a directory, and the file mode creation mask.
type ModeSpec = filemode.Spec

// ParseMode parses a mode operand, which is either a non-negative octal
// number or a symbolic mode expression.
func ParseMode(s string) (ModeSpec, error) {
	return filemode.Parse(s)
}

// modeString renders the permission and special bits of a mode like ls does,
// as in "rw-r--r--", with s, S, t and T in the execute columns.
func modeString(m fs.FileMode) string {
	var s [9]byte
	special := [3]fs.FileMode{fs.ModeSetuid, fs.ModeSetgid, fs.ModeSticky}
	for i, shift := range []uint{6, 3, 0} {
		perm := uint32(m.Perm()>>shift) & 7
		s[i*3] = bit('r', perm&4)
		s[i*3+1] = bit('w', perm&2)
		switch exec, sp := perm&1 != 0, m&special[i] != 0; {
		case exec && sp:
			s[i*3+2] = "sst"[i]
		case sp:
			s[i*3+2] = "SST"[i]
		case exec:
			s[i*3+2] = 'x'
		default:
			s[i*3+2] = '-'
		}
	}
	return string(s[:])
}

// bit returns the letter of a permission bit, or '-' when it's clear.
func bit(letter byte, on uint32) byte {
	if on != 0 {
		return letter
	}
	return '-'
}
