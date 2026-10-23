package diff

import (
	"bytes"
	"strings"
)

// SplitLines splits data into lines, keeping the trailing newline of each
// one, except for a last line without it when data doesn't end in a newline.
func SplitLines(data []byte) []string {
	var lines []string
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			lines = append(lines, string(data))
			break
		}
		lines = append(lines, string(data[:i+1]))
		data = data[i+1:]
	}
	return lines
}

// BlankKey is the key of diff's -b: it drops the trailing blanks of a line
// and maps every other run of blanks to a single space, so lines only
// differing in the amount of blanks compare equal. It takes lines as
// returned by SplitLines, with their trailing newline.
func BlankKey(line string) string {
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimRight(line, " \t")
	var out strings.Builder
	out.Grow(len(line))
	blank := false
	for i := range len(line) {
		switch c := line[i]; c {
		case ' ', '\t':
			blank = true
		default:
			if blank {
				out.WriteByte(' ')
				blank = false
			}
			out.WriteByte(c)
		}
	}
	return out.String()
}
