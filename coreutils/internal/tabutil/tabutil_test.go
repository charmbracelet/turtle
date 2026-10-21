package tabutil

import (
	"slices"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		tablist string
		want    Stops
		wantErr string
	}{
		{name: "a single number gives the distance", tablist: "8", want: Stops{step: 8}},
		{name: "a comma separated list", tablist: "4,8,12", want: Stops{list: []int{4, 8, 12}}},
		{name: "a blank separated list", tablist: "4 8 12", want: Stops{list: []int{4, 8, 12}}},
		{name: "mixed separators", tablist: "4, 8,\t12", want: Stops{list: []int{4, 8, 12}}},
		{name: "redundant separators", tablist: "4,, 8", want: Stops{list: []int{4, 8}}},
		{name: "a trailing separator keeps a single number a distance", tablist: "4,", want: Stops{step: 4}},
		{name: "an empty list", tablist: "", wantErr: "empty tab list"},
		{name: "only separators", tablist: " ,\t", wantErr: "empty tab list"},
		{name: "zero", tablist: "0", wantErr: "invalid tab stop: '0'"},
		{name: "a negative number", tablist: "-4", wantErr: "invalid tab stop: '-4'"},
		{name: "a non number", tablist: "x", wantErr: "invalid tab stop: 'x'"},
		{name: "a decimal number", tablist: "4.5", wantErr: "invalid tab stop: '4.5'"},
		{name: "a huge number", tablist: "999999999999999999999999", wantErr: "invalid tab stop: '999999999999999999999999'"},
		{name: "one invalid stop", tablist: "4,x", wantErr: "invalid tab stop: 'x'"},
		{name: "not ascending", tablist: "8,4", wantErr: "tab sizes must be ascending"},
		{name: "repeated stops", tablist: "4,4,8", wantErr: "tab sizes must be ascending"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stops, err := Parse(test.tablist)
			if err != nil {
				if err.Error() != test.wantErr {
					t.Fatalf("Parse(%q) error = %q, want %q", test.tablist, err, test.wantErr)
				}
				return
			}
			if test.wantErr != "" {
				t.Fatalf("Parse(%q) error = nil, want %q", test.tablist, test.wantErr)
			}
			if stops.step != test.want.step || !slices.Equal(stops.list, test.want.list) {
				t.Errorf("Parse(%q) = %v, want %v", test.tablist, stops, test.want)
			}
		})
	}
}

func TestNext(t *testing.T) {
	tests := []struct {
		name   string
		stops  Stops
		col    int
		want   int
		wantOK bool
	}{
		{name: "the default distance", stops: Default, col: 0, want: 8, wantOK: true},
		{name: "middle of a stop", stops: Default, col: 7, want: 8, wantOK: true},
		{name: "on a stop", stops: Default, col: 8, want: 16, wantOK: true},
		{name: "a distance of one", stops: Stops{step: 1}, col: 5, want: 6, wantOK: true},
		{name: "a list", stops: Stops{list: []int{4, 8, 12}}, col: 0, want: 4, wantOK: true},
		{name: "between list stops", stops: Stops{list: []int{4, 8, 12}}, col: 5, want: 8, wantOK: true},
		{name: "before the last list stop", stops: Stops{list: []int{4, 8, 12}}, col: 9, want: 12, wantOK: true},
		{name: "past the last list stop", stops: Stops{list: []int{4, 8, 12}}, col: 12, want: 0, wantOK: false},
		{name: "on the last list stop", stops: Stops{list: []int{4, 8}}, col: 8, want: 0, wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := test.stops.Next(test.col)
			if got != test.want || ok != test.wantOK {
				t.Errorf("Next(%d) = %d, %v, want %d, %v", test.col, got, ok, test.want, test.wantOK)
			}
		})
	}
}
