package usernames

import (
	"errors"
	"testing"
)

func TestResolve(t *testing.T) {
	// lookups stands for what the lookup of the identifier "1" gives: an
	// error, an empty name, or a name of its own.
	lookups := map[string]func(id string) (string, error){
		"missing": func(string) (string, error) { return "", errors.New("no") },
		"empty":   func(string) (string, error) { return "", nil },
		"named":   func(string) (string, error) { return "alice", nil },
	}

	tests := []struct {
		lookup       string
		unknown      string
		emptyMissing bool
		want         string
	}{
		// A failed lookup always takes the fallback: the decimal
		// identifier, or Unknown.
		{"missing", "", false, "1"},
		{"missing", "UNKNOWN", false, "UNKNOWN"},
		{"missing", "", true, "1"},
		{"missing", "UNKNOWN", true, "UNKNOWN"},
		// A name stat and find show what the lookup produced, even when
		// it's empty; ls falls back instead.
		{"empty", "", false, ""},
		{"empty", "UNKNOWN", false, ""},
		{"empty", "", true, "1"},
		{"empty", "UNKNOWN", true, "UNKNOWN"},
		// A name of its own is always used.
		{"named", "", false, "alice"},
		{"named", "UNKNOWN", false, "alice"},
		{"named", "", true, "alice"},
		{"named", "UNKNOWN", true, "alice"},
	}
	for _, test := range tests {
		for _, group := range []bool{false, true} {
			kind := "user"
			if group {
				kind = "group"
			}
			t.Run(test.lookup+" "+test.unknown+" "+kind, func(t *testing.T) {
				names := Names{
					Unknown:          test.unknown,
					EmptyNameMissing: test.emptyMissing,
				}
				if group {
					names.LookupGroup = lookups[test.lookup]
					if got := names.Group(1); got != test.want {
						t.Errorf("Group(1) = %q, want %q", got, test.want)
					}
					return
				}
				names.LookupUser = lookups[test.lookup]
				if got := names.User(1); got != test.want {
					t.Errorf("User(1) = %q, want %q", got, test.want)
				}
			})
		}
	}
}

func TestMemoized(t *testing.T) {
	calls := 0
	names := Names{
		LookupUser: func(id string) (string, error) {
			calls++
			return "alice", nil
		},
		LookupGroup: func(string) (string, error) {
			return "", errors.New("no")
		},
	}
	for range 3 {
		if got := names.User(1); got != "alice" {
			t.Errorf("User(1) = %q, want \"alice\"", got)
		}
	}
	if calls != 1 {
		t.Errorf("lookup called %d times, want 1", calls)
	}
	if got := names.Group(1); got != "1" {
		t.Errorf("Group(1) = %q, want \"1\"", got)
	}
}
