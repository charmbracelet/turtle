package grep

import (
	"strings"
)

// colors holds the escape sequences the --color option surrounds the
// matches and the prefixes with, as the GREP_COLORS environment variable
// describes them.
type colors struct {
	sl, cx, ms, mc, fn, ln, bn, se string
	rv, ne                         bool
}

// colorsOf returns the colors of a search, or nil when it stays uncolored.
func (p *Prog) colors(opts options) *colors {
	switch opts.color {
	case 0, 'n':
		return nil
	case 'a':
		if !p.Terminal(1) {
			return nil
		}
		if t := p.Getenv("TERM"); t == "" || t == "dumb" {
			return nil
		}
	}
	c := &colors{
		sl: "", cx: "", ms: "01;31", mc: "01;31",
		fn: "35", ln: "32", bn: "32", se: "36",
	}
	if spec, ok := p.LookupEnv("GREP_COLORS"); ok {
		c.parse(spec)
	} else if spec, ok := p.LookupEnv("GREP_COLOR"); ok {
		c.ms, c.mc = spec, spec
	}
	if c.rv && opts.invert {
		c.sl, c.cx = c.cx, c.sl
		c.ms, c.mc = c.mc, c.ms
	}
	return c
}

// parse reads the capabilities of a GREP_COLORS value.
func (c *colors) parse(spec string) {
	for item := range strings.SplitSeq(spec, ":") {
		name, value, given := strings.Cut(item, "=")
		if !given {
			switch name {
			case "rv":
				c.rv = true
			case "ne":
				c.ne = true
			}
			continue
		}
		switch name {
		case "sl":
			c.sl = value
		case "cx":
			c.cx = value
		case "ms":
			c.ms = value
		case "mc":
			c.mc = value
		case "mt":
			c.ms, c.mc = value, value
		case "fn":
			c.fn = value
		case "ln":
			c.ln = value
		case "bn":
			c.bn = value
		case "se":
			c.se = value
		}
	}
}

// start returns the escape sequence that begins a colored span.
func (c *colors) start(seq string) string {
	if c == nil {
		return ""
	}
	return "\x1b[" + seq + "m" + c.clear()
}

// fieldColor returns the sequence a prefix field is colored with: fn for a
// file name, ln for a line number and bn for a byte offset.
func (c *colors) fieldColor(which byte) string {
	if c == nil {
		return ""
	}
	switch which {
	case 'f':
		return c.fn
	case 'l':
		return c.ln
	}
	return c.bn
}

// matchColor returns the sequence the matched text of a line is colored
// with: ms in a selected line and mc in a context one.
func (c *colors) matchColor(selected bool) string {
	if c == nil {
		return ""
	}
	if selected {
		return c.ms
	}
	return c.mc
}

// end returns the escape sequence that closes a colored span.
func (c *colors) end() string {
	if c == nil {
		return ""
	}
	return "\x1b[m" + c.clear()
}

// clear returns the sequence that clears the rest of the line after a
// colored span, which the ne capability omits.
func (c *colors) clear() string {
	if c.ne {
		return ""
	}
	return "\x1b[K"
}

// colorize surrounds s with the escape sequence seq stands for.
func (c *colors) colorize(seq, s string) string {
	if c == nil || seq == "" || s == "" {
		return s
	}
	return c.start(seq) + s + c.end()
}

// sepColor colors a separator between the fields and the groups of lines.
func (c *colors) sepColor(s string) string {
	if c == nil {
		return s
	}
	return c.colorize(c.se, s)
}

// lineColor returns the sequence of a whole output line: sl for a selected
// one and cx for a context one.
func (c *colors) lineColor(selected bool) string {
	if c == nil {
		return ""
	}
	if selected {
		return c.sl
	}
	return c.cx
}
