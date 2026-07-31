package name

import "testing"

func BenchmarkNormalizeName(b *testing.B) {
	benchmarks := map[string][]string{
		"ascii": {"configuration"},
		"nfc":   {"naïve"},
		"nfd":   {"naïve"},
		"medium batch": {
			"configuration", "café", "naive-value", "déploiement", "ユーザー",
			"schema_name", "café", "данные", "δοκιμή", "服务端",
		},
	}

	for name, inputs := range benchmarks {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				for _, input := range inputs {
					_ = NormalizeName(input)
				}
			}
		})
	}
}
