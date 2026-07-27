package passive

import (
	"fmt"
	"strings"
	"testing"
)

// Representative of what the template scanner actually feeds SearchDatabaseErrors:
// the full raw response of a payload request against a modern JS app.
func benchBody(sizeKB int) string {
	var sb strings.Builder
	chunk := `<!DOCTYPE html><html><head><script src="/_next/static/chunks/main-app.js"></script>` +
		`<script>self.__next_f.push([1,"{\"props\":{\"pageProps\":{}},\"page\":\"/account/history\"}"])</script></head>` +
		`<body><div id="__next">balance 100.00 EUR</div></body></html>`
	for sb.Len() < sizeKB*1024 {
		sb.WriteString(chunk)
	}
	return "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n" + sb.String()
}

func BenchmarkSearchDatabaseErrors(b *testing.B) {
	for _, kb := range []int{4, 64, 256} {
		body := benchBody(kb)
		b.Run(fmt.Sprintf("clean-%dKB", kb), func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				if m := SearchDatabaseErrors(body); m != nil {
					b.Fatalf("unexpected match %v", m)
				}
			}
		})
	}
	hit := benchBody(64) + "\nPostgreSQL query failed: ERROR: parser: parse error at or near"
	b.Run("match-64KB", func(b *testing.B) {
		b.SetBytes(int64(len(hit)))
		for i := 0; i < b.N; i++ {
			if m := SearchDatabaseErrors(hit); m == nil {
				b.Fatal("expected match")
			}
		}
	})
}
