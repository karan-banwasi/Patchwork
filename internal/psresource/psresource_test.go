package psresource

import "testing"

func TestParsePSResourceJSON_ArrayAndSingle(t *testing.T) {
	arrayJSON := `[{"Name":"PSReadLine","Id":"PSReadLine","Version":"2.3.4","Available":"2.3.5","Source":"PSGallery"},{"Name":"Posh-Git","Id":"Posh-Git","Version":"1.0.0","Available":"1.1.0","Source":"PSGallery"}]`
	updates, err := parsePSResourceJSON(arrayJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}
	if updates[0].Id != "Posh-Git" || updates[0].ManagerName != "psresource" {
		t.Errorf("unexpected update[0]: %+v", updates[0])
	}

	singleJSON := `{"Name":"Terminal-Icons","Id":"Terminal-Icons","Version":"0.10.0","Available":"0.11.0","Source":"PSGallery"}`
	singleUpdates, err := parsePSResourceJSON(singleJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(singleUpdates) != 1 || singleUpdates[0].Id != "Terminal-Icons" {
		t.Errorf("unexpected single update result: %+v", singleUpdates)
	}
}
