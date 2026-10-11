package wire

import "testing"

func TestEnqueuesOf(t *testing.T) {
	text := "findings are strong\n\n@$ zone-tui-board: land the Arm A fixtures per the fence\n@$ ring-wraith: cross the ford with the findings\n@E 0\n"
	ords := EnqueuesOf(text)
	if len(ords) != 2 || ords[0].Agent != "zone-tui-board" || ords[0].Ask != "land the Arm A fixtures per the fence" {
		t.Fatalf("parsed: %+v", ords)
	}
	if ords[1].Agent != "ring-wraith" {
		t.Fatal("second order missing")
	}
	// lines that aren't orders ride no parser
	if EnqueuesOf("@$ missing colon\n@$ \nplain prose") != nil {
		t.Error("a single @$ with no agent:ask shape carries no order")
	}
}
