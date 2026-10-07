package proxy

import (
	"testing"

	"weave-os/router/internal/router/catalog"
)

// forcedReasoningEffort encodes the escalate-on-failure effort policy. No AIand
// roster model carries one of the prefixes that pin a forced level, so every
// roster model keeps adaptive per-model effort untouched ("") on a bare turn
// and on an escalated one alike.
func TestForcedReasoningEffort(t *testing.T) {
	if len(catalog.Models) == 0 {
		t.Fatal("catalog is empty")
	}
	for _, m := range catalog.Models {
		for _, escalate := range []bool{false, true} {
			if got := forcedReasoningEffort(m.ID, escalate); got != "" {
				t.Errorf("forcedReasoningEffort(%q, %v) = %q, want \"\"", m.ID, escalate, got)
			}
		}
	}
}
