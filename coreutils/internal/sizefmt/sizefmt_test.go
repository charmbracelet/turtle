package sizefmt

import "testing"

// TestHumanize checks the human readable sizes against the output of GNU
// coreutils 9.6.
func TestHumanize(t *testing.T) {
	for _, test := range []struct {
		n    int64
		si   bool
		want string
	}{
		{0, false, "0"},
		{500, false, "500"},
		{512, false, "512"},
		{1000, true, "1.0k"},
		{1023, false, "1023"},
		{1023, true, "1.1k"},
		{1024, false, "1.0K"},
		{1025, false, "1.1K"},
		{1050, false, "1.1K"},
		{1500, true, "1.5k"},
		{1536, false, "1.5K"},
		{10240, false, "10K"},
		{11264, false, "11K"},
		{100000, true, "100k"},
		{1048575, false, "1.0M"},
		{1048576, false, "1.0M"},
		{1048576, true, "1.1M"},
		{3145728, false, "3.0M"},
		{11264, true, "12k"},
	} {
		base := int64(1024)
		if test.si {
			base = 1000
		}
		if got := humanize(test.n, base); got != test.want {
			t.Errorf("humanize(%d, base %d) = %q, want %q", test.n, base, got, test.want)
		}
	}
}

// TestSpecFormat checks the sizes a block size spec writes.
func TestSpecFormat(t *testing.T) {
	for _, test := range []struct {
		spec Spec
		n    int64
		want string
	}{
		{Spec{}, 1500, "1500"},
		{ByteSpec, 1500, "1500"},
		{KibibyteSpec, 1500, "2"},
		{Fixed(512), 1024, "2"},
		{Fixed(0), 1024, "1024"},
		{Spec{unit: 1024}, 1024, "1"},
		{Spec{unit: 1024}, 1023, "1"},
		{Spec{unit: 1000000}, 1500, "1"},
		{Spec{unit: 1024, suffix: "KiB"}, 3000000, "2930KiB"},
		{Spec{unit: 1000, suffix: "kB"}, 3000000, "3000kB"},
		{Spec{unit: 1024, suffix: "K"}, 2048, "2K"},
		{HumanSpec(false), 2048, "2.0K"},
		{HumanSpec(true), 2048, "2.1k"},
	} {
		if got := test.spec.Format(test.n); got != test.want {
			t.Errorf("spec %+v Format(%d) = %q, want %q", test.spec, test.n, got, test.want)
		}
	}
}

// TestParse checks the block size words.
func TestParse(t *testing.T) {
	for _, test := range []struct {
		word string
		want Spec
	}{
		{"human-readable", HumanSpec(false)},
		{"si", HumanSpec(true)},
		{"1", Spec{unit: 1}},
		{"512", Spec{unit: 512}},
		{"10K", Spec{unit: 10240}},
		{"1MB", Spec{unit: 1000000}},
		{"1KiB", Spec{unit: 1024}},
		{"K", Spec{unit: 1024, suffix: "K"}},
		{"kB", Spec{unit: 1000, suffix: "kB"}},
		{"KiB", Spec{unit: 1024, suffix: "KiB"}},
		{"'10K", Spec{unit: 10240}},
	} {
		got, err := Parse(test.word)
		if err != nil {
			t.Errorf("Parse(%q) error = %v", test.word, err)
			continue
		}
		if got != test.want {
			t.Errorf("Parse(%q) = %+v, want %+v", test.word, got, test.want)
		}
	}
	for _, word := range []string{"", "0", "banana", "10Q1", "'"} {
		if _, err := Parse(word); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", word)
		}
	}
}
