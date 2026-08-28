package catalog_test

import (
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router/catalog"
)

func BenchmarkResolveBinding(b *testing.B) {
	available := map[string]struct{}{providers.ProviderAiand: {}}
	models := []string{
		"zai-org/glm-5.2",
		"moonshotai/kimi-k3",
		"motif-technologies/motif-3",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := models[i%len(models)]
		if _, ok := catalog.ResolveBinding(id, available); !ok {
			b.Fatalf("ResolveBinding(%q) miss", id)
		}
	}
}
