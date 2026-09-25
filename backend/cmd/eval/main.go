package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"tidalwave/backend/internal/clients"
	"tidalwave/backend/internal/config"
	"tidalwave/backend/internal/evalscore"
	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/pipeline/fake"
	"tidalwave/backend/internal/store"
)

type nopPublisher struct{}

func (nopPublisher) Publish(pipeline.Event) {}

func printCalibration(w io.Writer, table map[string]evalscore.CalibrationRow) {
	fmt.Fprintln(w, "\nJudgment calibration          cut   clean min/max (n)   flag min   note")
	keys := make([]string, 0, len(table))
	for name := range table {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	rangeText := func(r evalscore.ScoreRange) string {
		if r.Count == 0 {
			return "—"
		}
		return fmt.Sprintf("%.2f/%.2f (%d)", *r.Min, *r.Max, r.Count)
	}
	for _, name := range keys {
		row := table[name]
		note, flagMin := "", "—"
		if row.FalsePositiveRisk {
			note = "false-positive risk"
		}
		if row.Flag.Min != nil {
			flagMin = fmt.Sprintf("%.2f", *row.Flag.Min)
		}
		fmt.Fprintf(w, "%-28s  %.2f  %-19s %-10s %s\n", name, row.Threshold, rangeText(row.Clean), flagMin, note)
	}
}

func main() {
	testdata := flag.String("testdata", filepath.Join("..", "testdata"), "directory with fixtures/ and documents/")
	reportPath := flag.String("report", filepath.Join("..", "eval", "report.json"), "report output")
	useFake := flag.Bool("fake", false, "use fake stages (checks the harness, not the models)")
	flag.Parse()
	cfg := config.Load()
	session := fmt.Sprintf("eval-%d", time.Now().Unix())
	cfg.SpanboxSession = session
	var stages pipeline.Stages
	if *useFake {
		stages = fake.Stages()
	} else {
		var err error
		if stages, err = clients.RealStages(cfg); err != nil {
			log.Fatal(err)
		}
	}
	files, err := filepath.Glob(filepath.Join(*testdata, "fixtures", "*.json"))
	if err != nil || len(files) == 0 {
		log.Fatalf("no fixtures under %s/fixtures (run: go run ./cmd/gendata)", *testdata)
	}
	var fixtures []evalscore.Fixture
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			log.Fatal(err)
		}
		var fx evalscore.Fixture
		if err := json.Unmarshal(raw, &fx); err != nil {
			log.Fatalf("%s: %v", f, err)
		}
		fixtures = append(fixtures, fx)
	}
	tmp, err := os.MkdirTemp("", "harbor-eval")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	st, err := store.Open(filepath.Join(tmp, "eval.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	runner := pipeline.NewRunner(st, stages, nopPublisher{})
	var results []evalscore.CaseResult
	for _, fx := range fixtures {
		c, err := st.CreateCase(store.Case{BorrowerName: fx.ID, LoanNumber: fx.ID, LoanProduct: "eval", RequestedAmount: 1})
		if err != nil {
			log.Fatal(err)
		}
		fileByDoc := map[string]string{}
		for _, fd := range fx.Documents {
			abs, err := filepath.Abs(filepath.Join(*testdata, fd.File))
			if err != nil {
				log.Fatal(err)
			}
			d, err := st.CreateDocument(store.Document{CaseID: c.ID, FileName: filepath.Base(fd.File), FilePath: abs})
			if err != nil {
				log.Fatal(err)
			}
			fileByDoc[d.ID] = fd.File
			runner.Enqueue(c.ID, d.ID)
		}
		runner.Wait()
		detail, err := st.CaseDetail(c.ID)
		if err != nil {
			log.Fatal(err)
		}
		res := evalscore.CaseResult{FixtureID: fx.ID}
		for _, d := range detail.Documents {
			fields := map[string]any{}
			for _, f := range d.Fields {
				fields[f.Key] = f.Value
				if f.Flagged {
					res.Flagged = true
				}
			}
			doc := evalscore.DocResult{File: fileByDoc[d.ID], DocType: d.DocType, Fields: fields, Status: d.Status}
			for _, j := range d.Judgments {
				doc.Judgments = append(doc.Judgments, evalscore.JudgmentScore{Name: j.Name, Score: j.Score})
				if pipeline.IsLow(j.Name, j.Score) {
					res.Flagged = true
				}
			}
			res.Docs = append(res.Docs, doc)
		}
		for _, j := range detail.CaseJudgments {
			res.CaseJudgments = append(res.CaseJudgments, evalscore.JudgmentScore{Name: j.Name, Score: j.Score})
			if pipeline.IsLow(j.Name, j.Score) {
				res.Flagged = true
			}
		}
		if detail.Assessment != nil {
			res.DTI, res.Recommendation = detail.Assessment.DTI, detail.Assessment.Recommendation
		}
		results = append(results, res)
		log.Printf("%-18s %s", fx.ID, res.Recommendation)
	}
	rep := evalscore.Score(fixtures, results)
	rep.Session = session
	if err := os.MkdirAll(filepath.Dir(*reportPath), 0o755); err != nil {
		log.Fatal(err)
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*reportPath, raw, 0o644); err != nil {
		log.Fatal(err)
	}
	line := func(name string, l evalscore.Layer) {
		fmt.Printf("%-28s %3d/%-3d %5.1f%%\n", name, l.Correct, l.Total, l.Rate*100)
	}
	fmt.Printf("\nspanbox session: %s\n", session)
	line("Document classification", rep.Classification)
	line("Field extraction", rep.Extraction)
	line("End-to-end recommendation", rep.EndToEnd)
	line("Confidence calibration", rep.Calibration)
	printCalibration(os.Stdout, rep.CalibrationTable)
	for _, m := range rep.Misses {
		fmt.Printf("  miss  %-16s %-32s %-26s want %s, got %s\n", m.Fixture, m.File, m.What, m.Want, m.Got)
	}
	fmt.Printf("\nreport: %s\n", *reportPath)
}
