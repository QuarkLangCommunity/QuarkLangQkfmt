package main

// qkfmt 性能基准：扫描 + 排版（用合成大文件，约 1500 行）。

import (
	"strings"
	"testing"
)

func fmtBigSource(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("fn f")
		b.WriteString(strings.Repeat("x", i%5))
		b.WriteString(itoaFmt(i))
		b.WriteString(`(int n, String s) int {
    int total = 0;
    while (n > 0) {
        total = total + n;
        n = n - 1;
    }
    return total + s.size();
}

`)
	}
	return b.String()
}

func itoaFmt(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

func BenchmarkFormat(b *testing.B) {
	src := fmtBigSource(120)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := format(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScan(b *testing.B) {
	src := fmtBigSource(120)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := scan(src); err != nil {
			b.Fatal(err)
		}
	}
}
