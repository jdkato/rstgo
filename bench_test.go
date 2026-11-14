package rstgo

import (
	"os"
	"testing"
)

func BenchmarkConvertSpec(b *testing.B) {
	src, err := os.ReadFile("testdata/docutils/docs/ref/rst/restructuredtext.rst")
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		if _, err := Convert(src, Options{NoStylesheet: true}); err != nil {
			b.Fatal(err)
		}
	}
}
