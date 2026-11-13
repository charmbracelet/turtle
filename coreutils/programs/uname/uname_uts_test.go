//go:build linux

package uname

import (
	"testing"
)

func TestUtsFields(t *testing.T) {
	f, err := utsFields("TestOS")
	if err != nil {
		t.Fatalf("utsFields() error = %v", err)
	}
	if f.osName != "TestOS" {
		t.Errorf("utsFields() osName = %q, want %q", f.osName, "TestOS")
	}
	if f.sysname == "" || f.nodename == "" || f.release == "" || f.version == "" || f.machine == "" {
		t.Errorf("utsFields() = %+v, want every POSIX field set", f)
	}
	if f.hardwarePlatform != f.machine {
		t.Errorf("utsFields() hardware platform = %q, want the machine value %q",
			f.hardwarePlatform, f.machine)
	}
}
