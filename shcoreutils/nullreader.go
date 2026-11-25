package shcoreutils

import "io"

type nullReader struct{}

func (nullReader) Read([]byte) (int, error) { return 0, io.EOF }
