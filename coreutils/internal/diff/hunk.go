package diff

import (
	"io"
	"strconv"
	"strings"
)

// WriteHunk writes the hunk in the given format, without file headers, as
// reject files hold them.
func WriteHunk(w io.Writer, h Hunk, f PatchFormat) error {
	if f == ContextPatch {
		return writeHunkContext(w, h)
	}
	return writeHunkUnified(w, h)
}

// writeHunkUnified writes the hunk as a unified one, with its old and new
// lines interleaved.
func writeHunkUnified(w io.Writer, h Hunk) error {
	var b strings.Builder
	b.WriteString("@@ -")
	b.WriteString(hunkRange(h.FromLine, len(h.Old)))
	b.WriteString(" +")
	b.WriteString(hunkRange(h.ToLine, len(h.New)))
	b.WriteString(" @@\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}

	i, j := 0, 0
	for i < len(h.Old) || j < len(h.New) {
		switch {
		case i < len(h.Old) && h.Old[i].Kind == DeleteLine:
			if err := writeHunkLine(w, "-", h.Old[i]); err != nil {
				return err
			}
			i++
		case j < len(h.New) && h.New[j].Kind == InsertLine:
			if err := writeHunkLine(w, "+", h.New[j]); err != nil {
				return err
			}
			j++
		case i < len(h.Old) && j < len(h.New):
			// The old and new context lines pair up; write the old one.
			if err := writeHunkLine(w, " ", h.Old[i]); err != nil {
				return err
			}
			i, j = i+1, j+1
		default:
			l := h.Old[i]
			if i >= len(h.Old) {
				l = h.New[j]
				j++
			} else {
				i++
			}
			if err := writeHunkLine(w, " ", l); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeHunkContext writes the hunk as a copied context one, with its old
// and new lines in turn. Deleted and inserted lines come out with their
// plain marks, which reads the same to patchers as the changed ones.
func writeHunkContext(w io.Writer, h Hunk) error {
	if _, err := io.WriteString(w, "***************\n"); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("*** ")
	b.WriteString(hunkContextRange(h.FromLine, len(h.Old)))
	b.WriteString(" ****\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}
	for _, l := range h.Old {
		mark := "- "
		if l.Kind == ContextLine {
			mark = "  "
		}
		if err := writeHunkLine(w, mark, l); err != nil {
			return err
		}
	}
	b.Reset()
	b.WriteString("--- ")
	b.WriteString(hunkContextRange(h.ToLine, len(h.New)))
	b.WriteString(" ----\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}
	for _, l := range h.New {
		mark := "+ "
		if l.Kind == ContextLine {
			mark = "  "
		}
		if err := writeHunkLine(w, mark, l); err != nil {
			return err
		}
	}
	return nil
}

// writeHunkLine writes one line of a hunk with its mark, flagging the last
// line of a file that lacks a trailing newline.
func writeHunkLine(w io.Writer, mark string, l HunkLine) error {
	if _, err := io.WriteString(w, mark+l.Content); err != nil {
		return err
	}
	if l.NoNewline {
		_, err := io.WriteString(w, "\n\\ No newline at end of file\n")
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// hunkRange formats the 1-based start and count of a hunk side as the
// unified headers write them.
func hunkRange(start, count int) string {
	switch count {
	case 0:
		return strconv.Itoa(start) + ",0"
	case 1:
		return strconv.Itoa(start)
	default:
		return strconv.Itoa(start) + "," + strconv.Itoa(count)
	}
}

// hunkContextRange formats the 1-based start and count of a hunk side as
// the context headers write them.
func hunkContextRange(start, count int) string {
	switch count {
	case 0, 1:
		return strconv.Itoa(start)
	default:
		return strconv.Itoa(start) + "," + strconv.Itoa(start+count-1)
	}
}
