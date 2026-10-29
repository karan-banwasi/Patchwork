package npm

import "testing"

func TestParseNpmOutdatedJSON(t *testing.T) {
	sample := `{
  "typescript": {
    "current": "5.4.2",
    "wanted": "5.5.4",
    "latest": "5.5.4",
    "dependent": "global"
  },
  "@angular/cli": {
    "current": "17.0.0",
    "wanted": "17.3.0",
    "latest": "18.1.0",
    "dependent": "global"
  }
}`
	updates, err := parseNpmOutdatedJSON(sample)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}
	if updates[0].Id != "@angular/cli" || updates[0].Version != "17.0.0" || updates[0].AvailableVersion != "18.1.0" {
		t.Errorf("unexpected first update: %+v", updates[0])
	}
}
