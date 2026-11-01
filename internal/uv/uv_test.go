package uv

import "testing"

func TestParseUvToolList(t *testing.T) {
	sample := `ruff v0.6.1
- ruff.exe
httpie v3.2.2
- http.exe
- https.exe
black v24.4.2
- black.exe
`
	tools := parseUvToolList(sample)
	if len(tools) != 3 {
		t.Fatalf("expected 3 uv tools, got %d", len(tools))
	}
	if tools[0].Id != "ruff" || tools[0].Version != "0.6.1" {
		t.Errorf("unexpected tool[0]: %+v", tools[0])
	}
	if tools[1].Id != "httpie" || tools[1].Version != "3.2.2" {
		t.Errorf("unexpected tool[1]: %+v", tools[1])
	}
	if tools[2].Id != "black" || tools[2].Version != "24.4.2" {
		t.Errorf("unexpected tool[2]: %+v", tools[2])
	}
}
