package loop

import (
	"context"
	"testing"

	"github.com/Kaginari/agent-one/provider/mock"
)

// A turn that ends because the hand was lowered mid-drive is not a DONE the work earned — the
// end row's cause names the lexicon's word for the human.
func TestEndCauseNamesTheHumanOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := engine(t, mock.New(mock.Text("never reached")))
	r, err := e.Run(ctx, "x")
	if r == nil && err != nil {
		t.Fatalf("a pre-canceled turn still owes the chronicle its end row: %v", err)
	}
	var cause string
	for _, x := range events(t, e, r.RunID) {
		if x["t"] == "end" {
			cause, _ = x["cause"].(string)
		}
	}
	if cause != "the human" {
		t.Fatalf("a mid-drive hand-lower must journal its name: %q", cause)
	}
	// an earned close stays causeless — done turns don't invent the hand
	e2 := engine(t, mock.New(mock.Text("end")))
	r2, _ := e2.Run(context.Background(), "x")
	var cause2 string
	for _, x := range events(t, e2, r2.RunID) {
		if x["t"] == "end" {
			cause2, _ = x["cause"].(string)
		}
	}
	if cause2 != "" {
		t.Errorf("an earned DONE journals no cause, got %q", cause2)
	}
}
