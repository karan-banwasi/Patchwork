package dotnet

import "testing"

func TestParseDotnetToolListAndSearch(t *testing.T) {
	listOut := `Package Id                Version      Commands
---------------------------------------------------
dotnet-ef                 8.0.4        dotnet-ef
csharpier                 0.28.2       dotnet-csharpier
`
	tools := parseDotnetToolList(listOut)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Id != "dotnet-ef" || tools[0].Version != "8.0.4" {
		t.Errorf("unexpected tool 0: %+v", tools[0])
	}

	searchOut := `Package ID      Latest Version      Authors      Downloads      Verified
----------------------------------------------------------------------------
dotnet-ef       8.0.8               Microsoft    50000000       x
`
	latest := parseDotnetToolSearchLatest(searchOut, "dotnet-ef")
	if latest != "8.0.8" {
		t.Errorf("expected latest 8.0.8, got %q", latest)
	}
}
