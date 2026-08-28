//go:build smoke

package smoke

import (
	"net/http"
	"testing"
)

// TestSemanticCache verifies the cross-request semantic response cache on a
// live router: an identical non-streaming turn stores on miss (no x-router-cache
// header) and replays on repeat (x-router-cache: hit). Streaming is asserted to
// bypass the cache entirely.
func TestSemanticCache(t *testing.T) {
	t.Run("identical non-stream turn hits semantic cache on repeat", func(t *testing.T) {
		body := newRequest("smoke-semantic-cache").tokens(256).
			text("Reply with exactly the word: parity. Do not use tools.").
			build(t)

		miss := call(t, body)
		requireOKMessage(t, miss)
		if got := miss.headers.Get(headerRouterCache); got != "" {
			t.Errorf("first call: want no %s header (cache miss), got %q", headerRouterCache, got)
		}

		hit := call(t, body)
		requireOKMessage(t, hit)
		if got := hit.headers.Get(headerRouterCache); got != routerCacheHit {
			t.Errorf("second call: want %s=%q (cache hit), got %q", headerRouterCache, routerCacheHit, got)
		}
	})

	t.Run("streaming bypasses semantic cache", func(t *testing.T) {
		body := newRequest("smoke-semantic-cache-stream").tokens(256).streaming().
			text("Reply with exactly the word: stream. Do not use tools.").
			build(t)

		first := call(t, body)
		if first.status != http.StatusOK {
			t.Fatalf("stream first: want 200, got %d; body: %s", first.status, truncate(first.body, 400))
		}
		if got := first.headers.Get(headerRouterCache); got != "" {
			t.Errorf("streaming first call: want no %s header, got %q", headerRouterCache, got)
		}

		second := call(t, body)
		if second.status != http.StatusOK {
			t.Fatalf("stream second: want 200, got %d; body: %s", second.status, truncate(second.body, 400))
		}
		if got := second.headers.Get(headerRouterCache); got != "" {
			t.Errorf("streaming repeat: want no %s header (streaming never cached), got %q", headerRouterCache, got)
		}
	})
}
