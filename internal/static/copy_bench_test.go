package static_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroedin/alloy/internal/static"
)

// BenchmarkCopyStatic measures copy throughput for a directory with many files.
// Compare sequential vs concurrent implementation:
//
//	go test -bench=BenchmarkCopyStatic -benchmem ./internal/static/
func BenchmarkCopyStatic(b *testing.B) {
	for _, fileCount := range []int{100, 500, 2000} {
		b.Run(fmt.Sprintf("files=%d", fileCount), func(b *testing.B) {
			srcDir, err := os.MkdirTemp("", "bench-copy-src-*")
			if err != nil {
				b.Fatal(err)
			}
			defer os.RemoveAll(srcDir)

			// Create source files across 20 subdirectories
			content := make([]byte, 4096) // 4KB per file
			for i := range content {
				content[i] = byte(i % 256)
			}
			for i := 0; i < fileCount; i++ {
				subdir := filepath.Join(srcDir, fmt.Sprintf("dir-%d", i%20))
				if err := os.MkdirAll(subdir, 0755); err != nil {
				b.Fatal(err)
			}
				name := filepath.Join(subdir, fmt.Sprintf("file-%d.bin", i))
				if err := os.WriteFile(name, content, 0644); err != nil {
					b.Fatal(err)
				}
			}

			b.ResetTimer()
			for range b.N {
				dstDir, err := os.MkdirTemp("", "bench-copy-dst-*")
				if err != nil {
					b.Fatal(err)
				}
				if err := static.CopyStatic(srcDir, dstDir); err != nil {
					b.Fatal(err)
				}
				os.RemoveAll(dstDir)
			}
		})
	}
}
