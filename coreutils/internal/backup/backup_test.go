package backup

import (
	"testing"

	"charm.land/snail/fsx"
)

func TestParseMethod(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		want    Method
		wantErr bool
	}{
		{name: "full names", method: "numbered", want: Numbered},
		{name: "full name nil", method: "nil", want: Existing},
		{name: "full name never", method: "never", want: Simple},
		{name: "alias t", method: "t", want: Numbered},
		{name: "off", method: "off", want: None},
		{name: "prefix of one", method: "num", want: Numbered},
		{name: "prefix of existing", method: "ex", want: Existing},
		{name: "prefix of none", method: "no", want: None},
		{name: "prefix of nil", method: "ni", want: Existing},
		{name: "prefix of simple", method: "s", want: Simple},
		{name: "ambiguous prefix", method: "n", wantErr: true},
		{name: "unknown", method: "bogus", wantErr: true},
		{name: "empty", method: "", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseMethod(test.method)
			if test.wantErr {
				if err == nil {
					t.Fatalf("ParseMethod(%q) = %v, want an error", test.method, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMethod(%q) error = %v", test.method, err)
			}
			if got != test.want {
				t.Errorf("ParseMethod(%q) = %v, want %v", test.method, got, test.want)
			}
		})
	}
}

func TestName(t *testing.T) {
	tests := []struct {
		name  string
		files fsx.MemFS
		dest  string
		opts  Options
		want  string
		err   bool
	}{
		{
			name:  "no backups keep the name",
			files: fsx.MemFS{"f": {}},
			dest:  "f",
			opts:  Options{Method: None},
			want:  "f",
		},
		{
			name:  "simple backups take the suffix",
			files: fsx.MemFS{"f": {}, "f~": {}},
			dest:  "f",
			opts:  Options{Method: Simple, Suffix: "~"},
			want:  "f~",
		},
		{
			name: "a custom suffix",
			dest: "f",
			opts: Options{Method: Simple, Suffix: ".bak"},
			want: "f.bak",
		},
		{
			name: "numbered start at one",
			dest: "f",
			opts: Options{Method: Numbered},
			want: "f.~1~",
		},
		{
			name:  "numbered continue after the highest",
			files: fsx.MemFS{"f": {}, "f.~1~": {}, "f.~3~": {}},
			dest:  "f",
			opts:  Options{Method: Numbered},
			want:  "f.~4~",
		},
		{
			name:  "numbered in a directory",
			files: fsx.MemFS{"d/f": {}, "d/f.~2~": {}},
			dest:  "d/f",
			opts:  Options{Method: Numbered},
			want:  "d/f.~3~",
		},
		{
			name:  "existing falls back to simple",
			files: fsx.MemFS{"f": {}, "f~": {}},
			dest:  "f",
			opts:  Options{Method: Existing, Suffix: "~"},
			want:  "f~",
		},
		{
			name:  "existing follows numbered ones",
			files: fsx.MemFS{"f": {}, "f.~1~": {}},
			dest:  "f",
			opts:  Options{Method: Existing, Suffix: "~"},
			want:  "f.~2~",
		},
		{
			name:  "versions of other files don't count",
			files: fsx.MemFS{"f": {}, "g.~1~": {}, "ff.~1~": {}},
			dest:  "f",
			opts:  Options{Method: Existing},
			want:  "f~",
		},
		{
			name:  "numbered backups need the directory",
			files: fsx.MemFS{},
			dest:  "d/f",
			opts:  Options{Method: Numbered},
			err:   true,
		},
		{
			name:  "simple backups don't read the directory",
			files: fsx.MemFS{"f": {Data: []byte("x")}},
			dest:  "f",
			opts:  Options{Method: Simple, Suffix: "~"},
			want:  "f~",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fsys := test.files
			if fsys == nil {
				fsys = fsx.MemFS{}
			}
			opts := test.opts
			if opts.Method != None && opts.Suffix == "" {
				opts.Suffix = DefaultSuffix
			}
			got, err := Name(fsys, test.dest, opts)
			if test.err {
				if err == nil {
					t.Fatalf("Name(%q) = %q, want an error", test.dest, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Name(%q) error = %v", test.dest, err)
			}
			if got != test.want {
				t.Errorf("Name(%q) = %q, want %q", test.dest, got, test.want)
			}
		})
	}
}
