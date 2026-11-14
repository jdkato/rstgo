package rstgo

import (
	"os"
	"path/filepath"
	"testing"
)

// benchInputs are corpus documents of increasing size.
var benchInputs = []struct{ name, path string }{
	{"README", "testdata/docutils/README.rst"},
	{"FAQ", "testdata/docutils/FAQ.rst"},
	{"Spec", "testdata/docutils/docs/ref/rst/restructuredtext.rst"},
	{"History", "testdata/docutils/HISTORY.rst"},
}

func BenchmarkConvert(b *testing.B) {
	for _, in := range benchInputs {
		src, err := os.ReadFile(filepath.FromSlash(in.path))
		if err != nil {
			b.Fatal(err)
		}
		b.Run(in.name, func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := Convert(src, Options{NoStylesheet: true}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
