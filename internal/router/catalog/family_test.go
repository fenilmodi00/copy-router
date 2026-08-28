package catalog

import "testing"

func TestFamilyAndVersion(t *testing.T) {
	tests := []struct {
		id      string
		family  string
		version [2]int
		ok      bool
	}{
		{"zai-org/glm-5.2", "zai-org/glm", [2]int{5, 2}, true},
		{"moonshotai/kimi-k2.7", "moonshotai/kimi-k", [2]int{2, 7}, true},
		{"moonshotai/kimi-k3", "moonshotai/kimi-k", [2]int{3, 0}, true},
		{"deepseek-ai/deepseek-v4-flash", "deepseek-ai/deepseek-v-flash", [2]int{4, 0}, true},
		{"deepseek-ai/deepseek-v4-pro", "deepseek-ai/deepseek-v-pro", [2]int{4, 0}, true},
		{"openai/gpt-oss-120b", "", [2]int{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			family, version, ok := FamilyAndVersion(tt.id)
			if ok != tt.ok {
				t.Fatalf("FamilyAndVersion(%q) ok = %v, want %v", tt.id, ok, tt.ok)
			}
			if !ok {
				return
			}
			if family != tt.family {
				t.Errorf("FamilyAndVersion(%q) family = %q, want %q", tt.id, family, tt.family)
			}
			if version != tt.version {
				t.Errorf("FamilyAndVersion(%q) version = %v, want %v", tt.id, version, tt.version)
			}
		})
	}
}

func TestFamilyDuplicates(t *testing.T) {
	ids := []string{
		"moonshotai/kimi-k2.7",
		"moonshotai/kimi-k3",
		"zai-org/glm-5.2",
	}
	dups := FamilyDuplicates(ids)
	got := make(map[string]string, len(dups))
	for _, d := range dups {
		got[d.Superseded] = d.SupersededBy
	}
	want := map[string]string{
		"moonshotai/kimi-k2.7": "moonshotai/kimi-k3",
	}
	if len(got) != len(want) {
		t.Fatalf("FamilyDuplicates(%v) = %v, want %v", ids, dups, want)
	}
	for supersededID, wantBy := range want {
		gotBy, ok := got[supersededID]
		if !ok {
			t.Errorf("expected %q to be flagged as superseded, was not", supersededID)
			continue
		}
		if gotBy != wantBy {
			t.Errorf("FamilyDuplicates: %q superseded by %q, want %q", supersededID, gotBy, wantBy)
		}
	}
}

func TestFamilyDuplicates_NoFalsePositiveOnDistinctSizes(t *testing.T) {
	ids := []string{
		"deepseek-ai/deepseek-v4-flash",
		"deepseek-ai/deepseek-v4-pro",
	}
	if dups := FamilyDuplicates(ids); len(dups) != 0 {
		t.Errorf("FamilyDuplicates(%v) = %v, want no duplicates", ids, dups)
	}
}
