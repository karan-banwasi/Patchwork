package version

import "testing"

func FuzzCompare(f *testing.F) {
	seeds := []struct {
		eco string
		v1  string
		v2  string
	}{
		{"generic", "1.0.0", "1.0.1"},
		{"generic", "1.2.3+build.1", "1.2.3+build.2"},
		{"generic", "2.0.0-rc.1", "2.0.0"},
		{"uv", "1!2.0.0a1", "1!2.0.0"},
		{"uv", "1.0.0.post1", "1.0.0"},
		{"apt", "1:2.3-4ubuntu1", "1:2.3-5ubuntu1"},
		{"brew", "1.16.0_1", "1.16.0_2"},
		{"npm", "v18.0.0", "18.0.1"},
		{"winget", "2.44.0.0", "2.45.0.0"},
		{"", "unknown", "1.0.0"},
		{"", "dev", ""},
		{"", "", ""},
	}
	for _, s := range seeds {
		f.Add(s.eco, s.v1, s.v2)
	}

	f.Fuzz(func(t *testing.T, eco, v1, v2 string) {
		if len(eco) > 64 || len(v1) > 256 || len(v2) > 256 {
			return
		}

		cmp := Compare(eco, v1, v2)
		if cmp != -1 && cmp != 0 && cmp != 1 {
			t.Fatalf("Compare(%q, %q, %q) = %d, want -1, 0, or 1", eco, v1, v2, cmp)
		}
		if selfCmp := Compare(eco, v1, v1); selfCmp != 0 {
			t.Fatalf("Compare(%q, %q, %q) = %d, want 0", eco, v1, v1, selfCmp)
		}
		if IsUpgrade(eco, v1, v1) {
			t.Fatalf("IsUpgrade(%q, %q, %q) = true, want false", eco, v1, v1)
		}
		_ = IsUpgrade(eco, v1, v2)
	})
}

func FuzzEvaluateConstraint(f *testing.F) {
	seeds := []struct {
		eco        string
		ver        string
		constraint string
	}{
		{"generic", "1.2.3", ">=1.0.0"},
		{"generic", "1.2.3", "<2.0.0"},
		{"generic", "1.2.3", "^1.2.0"},
		{"generic", "1.2.3", "~1.2.0"},
		{"npm", "2.1.0", ">=1.0.0, <2.0.0"},
		{"uv", "1!2.0.0rc1", "==1!2.0.0rc1"},
		{"generic", "1.0.0", "*"},
		{"generic", "1.0.0", ""},
		{"generic", "1.0.0", "not-valid"},
	}
	for _, s := range seeds {
		f.Add(s.eco, s.ver, s.constraint)
	}

	f.Fuzz(func(t *testing.T, eco, ver, constraint string) {
		if len(eco) > 64 || len(ver) > 256 || len(constraint) > 256 {
			return
		}
		_, _ = EvaluateConstraint(eco, ver, constraint)
	})
}
