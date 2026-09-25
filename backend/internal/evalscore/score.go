package evalscore

import (
	"fmt"
	"math"
	"strings"

	"tidalwave/backend/internal/pipeline"
)

type Expected struct {
	DTI            *float64 `json:"dti"`
	Recommendation string   `json:"recommendation"`
	ShouldFlag     bool     `json:"should_flag_low_confidence"`
}
type FixtureDoc struct {
	File            string         `json:"file"`
	ExpectedDocType string         `json:"expected_doc_type"`
	ExpectedFields  map[string]any `json:"expected_fields"`
}
type Fixture struct {
	ID          string       `json:"id"`
	Description string       `json:"description"`
	Documents   []FixtureDoc `json:"documents"`
	Expected    Expected     `json:"expected"`
}
type JudgmentScore struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
}
type DocResult struct {
	File      string          `json:"file"`
	DocType   string          `json:"doc_type"`
	Fields    map[string]any  `json:"fields"`
	Status    string          `json:"status"`
	Judgments []JudgmentScore `json:"judgments"`
}
type CaseResult struct {
	FixtureID      string          `json:"fixture_id"`
	Docs           []DocResult     `json:"docs"`
	DTI            *float64        `json:"dti"`
	Recommendation string          `json:"recommendation"`
	Flagged        bool            `json:"flagged"`
	CaseJudgments  []JudgmentScore `json:"case_judgments"`
}
type Layer struct {
	Correct int     `json:"correct"`
	Total   int     `json:"total"`
	Rate    float64 `json:"rate"`
}
type Miss struct {
	Fixture string `json:"fixture"`
	File    string `json:"file,omitempty"`
	What    string `json:"what"`
	Want    string `json:"want"`
	Got     string `json:"got"`
}
type ScoreRange struct {
	Min   *float64 `json:"min"`
	Max   *float64 `json:"max"`
	Count int      `json:"count"`
}
type CalibrationRow struct {
	Threshold         float64    `json:"threshold"`
	Clean             ScoreRange `json:"clean"`
	Flag              ScoreRange `json:"flag"`
	FalsePositiveRisk bool       `json:"false_positive_risk"`
}
type Report struct {
	Session          string                    `json:"session"`
	Classification   Layer                     `json:"classification"`
	Extraction       Layer                     `json:"extraction"`
	EndToEnd         Layer                     `json:"end_to_end"`
	Calibration      Layer                     `json:"calibration"`
	CalibrationTable map[string]CalibrationRow `json:"calibration_table"`
	Misses           []Miss                    `json:"misses"`
	Results          []CaseResult              `json:"results"`
}

func (l *Layer) add(ok bool) {
	l.Total++
	if ok {
		l.Correct++
	}
	l.Rate = float64(l.Correct) / float64(l.Total)
}
func sameValue(want, got any) bool {
	if w, ok := want.(float64); ok {
		g, ok := got.(float64)
		return ok && math.Abs(w-g) <= 1.0
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(want)), strings.TrimSpace(fmt.Sprint(got)))
}
func fmtDTI(d *float64) string {
	if d == nil {
		return "none"
	}
	return fmt.Sprintf("%.4f", *d)
}
func (r *ScoreRange) add(score float64) {
	if r.Min == nil || score < *r.Min {
		r.Min = &score
	}
	if r.Max == nil || score > *r.Max {
		r.Max = &score
	}
	r.Count++
}

func Score(fixtures []Fixture, results []CaseResult) Report {
	byID := map[string]CaseResult{}
	for _, r := range results {
		byID[r.FixtureID] = r
	}
	rep := Report{Misses: []Miss{}, Results: results, CalibrationTable: map[string]CalibrationRow{}}
	for _, fx := range fixtures {
		res := byID[fx.ID]
		docs := map[string]DocResult{}
		addScore := func(j JudgmentScore) {
			row := rep.CalibrationTable[j.Name]
			row.Threshold = pipeline.Threshold(j.Name)
			if fx.Expected.ShouldFlag {
				row.Flag.add(j.Score)
			} else {
				row.Clean.add(j.Score)
			}
			rep.CalibrationTable[j.Name] = row
		}
		for _, d := range res.Docs {
			docs[d.File] = d
			for _, j := range d.Judgments {
				addScore(j)
			}
		}
		for _, j := range res.CaseJudgments {
			addScore(j)
		}
		for _, fd := range fx.Documents {
			got := docs[fd.File]
			ok := got.DocType == fd.ExpectedDocType
			rep.Classification.add(ok)
			if !ok {
				rep.Misses = append(rep.Misses, Miss{fx.ID, fd.File, "doc_type", fd.ExpectedDocType, got.DocType})
			}
			if fd.ExpectedDocType == "other" {
				continue
			}
			for k, want := range fd.ExpectedFields {
				v, present := got.Fields[k]
				ok := present && sameValue(want, v)
				rep.Extraction.add(ok)
				if !ok {
					rep.Misses = append(rep.Misses, Miss{fx.ID, fd.File, "field " + k, fmt.Sprint(want), fmt.Sprint(v)})
				}
			}
		}
		dtiOK := (fx.Expected.DTI == nil && res.DTI == nil) || (fx.Expected.DTI != nil && res.DTI != nil && math.Abs(*fx.Expected.DTI-*res.DTI) <= 0.005)
		e2e := res.Recommendation == fx.Expected.Recommendation && dtiOK
		rep.EndToEnd.add(e2e)
		if !e2e {
			rep.Misses = append(rep.Misses, Miss{Fixture: fx.ID, What: "recommendation/dti", Want: fx.Expected.Recommendation + " @ " + fmtDTI(fx.Expected.DTI), Got: res.Recommendation + " @ " + fmtDTI(res.DTI)})
		}
		cal := res.Flagged == fx.Expected.ShouldFlag
		rep.Calibration.add(cal)
		if !cal {
			rep.Misses = append(rep.Misses, Miss{Fixture: fx.ID, What: "low-confidence flag", Want: fmt.Sprint(fx.Expected.ShouldFlag), Got: fmt.Sprint(res.Flagged)})
		}
	}
	for name, row := range rep.CalibrationTable {
		row.FalsePositiveRisk = row.Clean.Min != nil && *row.Clean.Min < row.Threshold
		rep.CalibrationTable[name] = row
	}
	return rep
}
