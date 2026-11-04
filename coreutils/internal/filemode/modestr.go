package filemode

import (
	"io/fs"
	"strings"
)

// String returns the type and mode bits of a file in symbolic form, as ls
// writes them in its long format, like "drwxr-xr-x".
func String(m fs.FileMode) string {
	var b strings.Builder
	b.WriteByte(TypeChar(m))
	b.WriteString(class(m, 0o400, 0o200, 0o100, fs.ModeSetuid, 's', 'S'))
	b.WriteString(class(m, 0o040, 0o020, 0o010, fs.ModeSetgid, 's', 'S'))
	sticky := fs.ModeSticky
	if m&fs.ModeDir == 0 {
		// The sticky flag is only written for directories.
		sticky = 0
	}
	b.WriteString(class(m, 0o004, 0o002, 0o001, sticky, 't', 'T'))
	return b.String()
}

// TypeChar returns the character that describes the type of a file: 'd', 'l',
// 'p', 's', 'c', 'b' or '-' for a regular file.
func TypeChar(m fs.FileMode) byte {
	switch {
	case m&fs.ModeDir != 0:
		return 'd'
	case m&fs.ModeSymlink != 0:
		return 'l'
	case m&fs.ModeNamedPipe != 0:
		return 'p'
	case m&fs.ModeSocket != 0:
		return 's'
	case m&fs.ModeDevice != 0 && m&fs.ModeCharDevice != 0:
		return 'c'
	case m&fs.ModeDevice != 0:
		return 'b'
	case m&fs.ModeIrregular != 0:
		return '?'
	}
	return '-'
}

// class returns the three permission characters of one class, with the flag
// of the class in the third position when it's set.
func class(m, read, write, execute, flag fs.FileMode, set, unset byte) string {
	chars := []byte{'-', '-', '-'}
	if m&read != 0 {
		chars[0] = 'r'
	}
	if m&write != 0 {
		chars[1] = 'w'
	}
	switch {
	case m&execute != 0:
		chars[2] = 'x'
	case m&flag != 0:
		chars[2] = unset
	}
	if m&execute != 0 && m&flag != 0 {
		chars[2] = set
	}
	return string(chars)
}
