package occur

import (
	"bytes"
	"fmt"
	"testing"
)

// Dense matches exercise line-number calculation as well as match finding.
func BenchmarkLines(b *testing.B) {
	for _, tc := range []struct{ size, matches int }{
		{16 * 1024, 1},
		{1024 * 1024, 1},
		{1024 * 1024, 100},
		{1024 * 1024, 1000},
		{1024 * 1024, 10000},
	} {
		b.Run(fmt.Sprintf("bytes=%d/matches=%d", tc.size, tc.matches), func(b *testing.B) {
			content := bytes.Repeat([]byte("x"), tc.size)
			for i := 79; i < len(content); i += 80 {
				content[i] = '\n'
			}
			old := []byte("@match")
			for i := range tc.matches {
				copy(content[(i+1)*len(content)/(tc.matches+1):], old)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(content)))
			for b.Loop() {
				if got := len(Lines(content, old)); got != tc.matches {
					b.Fatalf("got %d matches, want %d", got, tc.matches)
				}
			}
		})
	}
}
