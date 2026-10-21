// Package tabutil implements the tab stop handling shared by the expand and
// unexpand utilities: parsing the tablist option-argument and walking the tab
// stops of a line.
package tabutil

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Stops holds the tab stops of a line, counted in columns from its start: a
// tab found on a column below a stop lands on that stop's column.
type Stops struct {
	// step is the distance between tab stops, when positive. Zero means the
	// stops are the ones of the list.
	step int

	// list holds tab stop positions in ascending order, used when step is
	// zero.
	list []int
}

// Default holds the default tab stops: every 8 columns.
var Default = Stops{step: 8}

// Parse parses a tablist option-argument: either a single positive integer,
// which sets tab stops that many columns apart, or two or more positive
// integers separated by commas or blanks, which set tab stops at those
// specific columns.
func Parse(tablist string) (Stops, error) {
	fields := strings.FieldsFunc(tablist, func(r rune) bool {
		return r == ' ' || r == '\t' || r == ','
	})
	stops := make([]int, 0, len(fields))
	for _, field := range fields {
		n, err := parseStop(field)
		if err != nil {
			return Stops{}, err
		}
		stops = append(stops, n)
	}
	switch len(stops) {
	case 0:
		return Stops{}, errors.New("empty tab list")
	case 1:
		return Stops{step: stops[0]}, nil
	}
	for i, stop := range stops[1:] {
		if stop <= stops[i] {
			return Stops{}, errors.New("tab sizes must be ascending")
		}
	}
	return Stops{list: stops}, nil
}

// parseStop parses a positive decimal tab stop position.
func parseStop(s string) (int, error) {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("invalid tab stop: '%s'", s)
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid tab stop: '%s'", s)
	}
	return n, nil
}

// Next returns the column a tab found on column col lands on, reporting false
// when col is at or past the last stop of a list.
func (s Stops) Next(col int) (int, bool) {
	if s.step > 0 {
		return col - col%s.step + s.step, true
	}
	for _, stop := range s.list {
		if stop > col {
			return stop, true
		}
	}
	return 0, false
}
