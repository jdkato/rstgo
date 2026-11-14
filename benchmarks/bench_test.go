// Package benchmarks compares rstgo with goldmark on the same generated
// document written in reStructuredText and Markdown. It is a separate module so
// that the main module stays free of dependencies.
package benchmarks

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/jdkato/rstgo"
	"github.com/yuin/goldmark"
)

var sizes = []int{8, 80, 800} // sections; about 15 KB, 150 KB, 1.5 MB

func BenchmarkConvert(b *testing.B) {
	for _, n := range sizes {
		md, ad := []byte(Gen("md", n)), []byte(Gen("rst", n))
		b.Run(fmt.Sprintf("goldmark/%d", n), func(b *testing.B) {
			b.SetBytes(int64(len(md)))
			b.ReportAllocs()
			gm := goldmark.New()
			for i := 0; i < b.N; i++ {
				var buf bytes.Buffer
				if err := gm.Convert(md, &buf); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("rstgo/%d", n), func(b *testing.B) {
			b.SetBytes(int64(len(ad)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := rstgo.ConvertBody(ad, rstgo.Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
