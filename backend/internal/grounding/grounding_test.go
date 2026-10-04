package grounding

import (
	"strings"
	"testing"
)

const textPage = `{"page_number":2,"page_width":612,"page_height":792,"success":true,"items":[{"md":"é Jordan Alvarez $1,200.00","grounding":{"lines":[{"words":[{"span":[3,9],"bbox":{"x":10,"y":20,"w":30,"h":10}},{"span":[10,17],"bbox":{"x":45,"y":20,"w":40,"h":10}},{"span":[18,27],"bbox":{"x":90,"y":20,"w":55,"h":10}}]}]}}]}`
const tablePage = `{"page_number":1,"page_width":612,"page_height":792,"success":true,"items":[{"md":"| Label | Value |\n| --- | :---: |\n| A\\|B | $1,200.00 |\n| Other | 98 |","grounding":{"rows":[[{"span":[0,5],"bbox":[{"x":10,"y":10,"w":20,"h":10}]},{"span":[0,5],"bbox":[{"x":50,"y":10,"w":20,"h":10}]}],[{"span":[0,4],"bbox":[{"x":10,"y":30,"w":20,"h":10}]},{"span":[0,9],"bbox":[{"x":50,"y":30,"w":40,"h":10},{"x":50,"y":40,"w":30,"h":10}]}],[{"span":[0,5],"bbox":[{"x":10,"y":60,"w":20,"h":10}]},{"span":[0,2],"bbox":[{"x":50,"y":60,"w":20,"h":10}]}]]}}]}`

func TestGroundingFailureCasesAndCoordinates(t *testing.T) {
	for _, tt := range []struct {
		name, sidecar string
		value         any
		want          *Box
	}{
		{"no hit", textPage, "missing", nil},
		{"multiple pages", textPage + "\n" + textPage, "Jordan", nil},
		{"multiple hits in cell", strings.Replace(tablePage, "$1,200.00", "98 98 980", 1), "98", nil},
		{"page failed", strings.Replace(textPage, `"success":true`, `"success":false`, 1), "Jordan", nil},
		{"missing sidecar", "", "Jordan", nil},
		{"table row misalignment", strings.Replace(tablePage, "| Other | 98 |", "", 1), 1200.0, nil},
		{"table column misalignment", strings.Replace(tablePage, "| Other | 98 |", "| Other |", 1), 1200.0, nil},
		{"invalid byte span", strings.Replace(textPage, `[3,9]`, `[1,9]`, 1), "Jordan", nil},
		{"UTF8 and multiword", textPage, "jordan alvarez", &Box{Page: 2, X: 10, Y: 20, W: 75, H: 10, PageWidth: 612, PageHeight: 792}},
		{"number formatting", textPage, 1200.0, &Box{Page: 2, X: 90, Y: 20, W: 55, H: 10, PageWidth: 612, PageHeight: 792}},
		{"cell-relative spans and escaped pipes", tablePage, 1200.0, &Box{Page: 1, X: 50, Y: 30, W: 40, H: 20, PageWidth: 612, PageHeight: 792}},
		{"skip table separator", tablePage, 98.0, &Box{Page: 1, X: 50, Y: 60, W: 20, H: 10, PageWidth: 612, PageHeight: 792}},
		{"no partial numeric match", textPage, 200.0, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := Decode(strings.NewReader(tt.sidecar))
			if err != nil {
				t.Fatal(err)
			}
			got := Match(tokens, tt.value)
			if (got == nil) != (tt.want == nil) || got != nil && *got != *tt.want {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
		})
	}
	if _, err := Decode(strings.NewReader("not JSON")); err == nil {
		t.Fatal("malformed sidecar accepted")
	}
}
