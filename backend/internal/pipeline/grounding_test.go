package pipeline_test

import (
	"context"
	"testing"

	"tidalwave/backend/internal/grounding"
	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/pipeline/fake"
)

func TestGroundingPersistsAndEditClearsIt(t *testing.T) {
	s, d := setup(t, "bank-statement.pdf")
	if err := pipeline.Process(context.Background(), s, fake.Stages(), &recorder{}, d); err != nil {
		t.Fatal(err)
	}
	detail, err := s.CaseDetail(d.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	var box *grounding.Box
	for _, f := range detail.Documents[0].Fields {
		if f.Key == "ending_balance" {
			box = f.Source
		}
	}
	if box == nil || box.Page != 1 || box.PageWidth <= 0 {
		t.Fatalf("no stored fake grounding: %+v", box)
	}
	if _, err := s.UpdateField(d.ID, "ending_balance", 99.0); err != nil {
		t.Fatal(err)
	}
	detail, err = s.CaseDetail(d.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range detail.Documents[0].Fields {
		if f.Key == "ending_balance" && f.Source != nil {
			t.Fatal("edit retained stale grounding")
		}
	}
}
