package main

import (
	"bytes"
	"testing"
)

func TestNewestKeepsTheEnd(t *testing.T) {
	var out newest
	for _, b := range []byte("abc") {
		_, _ = out.Write(bytes.Repeat([]byte{b}, 40<<10))
	}
	if len(out) != 64<<10 || !bytes.Equal(out[24<<10:], bytes.Repeat([]byte("c"), 40<<10)) {
		t.Fatalf("kept %d bytes ending %q", len(out), out[len(out)-3:])
	}
}
