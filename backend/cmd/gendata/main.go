package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-pdf/fpdf"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"tidalwave/backend/internal/schemas"
)

type fixtureDoc struct {
	File            string         `json:"file"`
	ExpectedDocType string         `json:"expected_doc_type"`
	ExpectedFields  map[string]any `json:"expected_fields"`
}
type fixture struct {
	ID          string       `json:"id"`
	Description string       `json:"description"`
	Documents   []fixtureDoc `json:"documents"`
	Expected    expected     `json:"expected"`
}

const footer = "SYNTHETIC TEST DOCUMENT - NOT A REAL RECORD"

var titles = map[string]string{
	"w2": "Form W-2  Wage and Tax Statement  2025", "form_1040": "Form 1040  U.S. Individual Income Tax Return  2025",
	"form_1003": "Uniform Residential Loan Application (Form 1003)", "pay_stub": "Earnings Statement",
	"bank_statement": "Checking Account Statement", "other": "STATE OF NEW JERSEY  DRIVER LICENSE",
}

func lines(d docSpec) []string {
	out := []string{titles[d.Type], "", "Borrower: " + d.Holder}
	if d.Type == "other" {
		return append(out, "Name: "+d.Holder, "DOB: 04/11/1990", "Class: D", "Expires: 04/11/2030", "ID: S1234 56789 01234")
	}
	for _, spec := range schemas.Registry[d.Type] {
		v := d.Fields[spec.Key]
		switch spec.Kind {
		case schemas.KindNumber:
			out = append(out, fmt.Sprintf("%-34s $%s", spec.Label, commas(v.(float64))))
		case schemas.KindInt:
			out = append(out, fmt.Sprintf("%-34s %d", spec.Label, int(v.(float64))))
		default:
			out = append(out, fmt.Sprintf("%-34s %s", spec.Label, v))
		}
	}
	if d.Type == schemas.PayStub {
		out = append(out, "Pay frequency: Semimonthly", "Pay periods to date: 16",
			fmt.Sprintf("%-34s $%s", "Net pay this period", commas(d.Fields["gross_pay"].(float64)*0.76)))
	}
	if d.Type == schemas.BankStatement {
		deposit := d.Fields["total_deposits"].(float64) / 2
		debt := d.Fields["monthly_debt"].(float64)
		bnpl := int(d.Fields["bnpl_hits"].(float64))
		utilities := d.Fields["total_withdrawals"].(float64) - debt - float64(bnpl)*45 - 540.25
		entry := func(date, payee, sign string, amount float64) string {
			return fmt.Sprintf("%s | %s | %s$%s", date, payee, sign, commas(amount))
		}
		out = append(out, fmt.Sprintf("Payroll deposits (2 x $%s net)", commas(deposit)), "", "Transaction register (August 2026)",
			entry("08/02", "Example Payroll Deposit", "+", deposit),
			entry("08/03", "Example Auto Loan (debt)", "-", 600),
			entry("08/05", "Sample Student Loan (debt)", "-", 400),
			entry("08/07", "Demo Card Minimum (debt)", "-", 300),
			entry("08/08", "Example Housing (debt)", "-", debt-1300))
		for i := 0; i < bnpl; i++ {
			out = append(out, entry(fmt.Sprintf("08/%02d", 9+i), fmt.Sprintf("Sample BNPL Plan %d", i+1), "-", 45))
		}
		out = append(out,
			entry("08/13", "Demo Groceries", "-", 540.25),
			entry("08/16", "Example Payroll Deposit", "+", deposit),
			entry("08/21", "Sample Utilities", "-", utilities))
	}
	return out
}
func commas(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	intPart, dec := s[:len(s)-3], s[len(s)-3:]
	var b bytes.Buffer
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String() + dec
}
func keywords(pdf *fpdf.Fpdf, d docSpec) error {
	fields := map[string]any{"doc_type": d.Type}
	for k, v := range d.Fields {
		fields[k] = v
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	pdf.SetKeywords("HARBOR-FIELDS:"+string(raw), false)
	return nil
}

func textPDF(path string, d docSpec) error {
	body := lines(d)
	pdf := fpdf.New("P", "mm", "Letter", "")
	pdf.SetCompression(false)
	if err := keywords(pdf, d); err != nil {
		return err
	}
	pdf.AddPage()
	pdf.SetFont("Courier", "B", 13)
	pdf.CellFormat(0, 10, body[0], "", 1, "L", false, 0, "")
	pdf.SetFont("Courier", "", 11)
	for _, l := range body[1:] {
		if d.Tampered && strings.HasPrefix(l, "Ending balance") {
			pdf.SetFont("Helvetica", "B", 14)
		}
		pdf.CellFormat(0, 7, l, "", 1, "L", false, 0, "")
		if d.Tampered && strings.HasPrefix(l, "Ending balance") {
			pdf.SetFont("Courier", "", 11)
		}
	}
	pdf.SetY(-20)
	pdf.SetFont("Courier", "I", 8)
	pdf.CellFormat(0, 6, footer, "", 0, "C", false, 0, "")
	return pdf.OutputFileAndClose(path)
}
func endingDigitBounds(body []string) image.Rectangle {
	for i, l := range body {
		if strings.HasPrefix(l, "Ending balance") {
			x := 12 + (strings.Index(l, "$")+5)*7
			y := 18 + i*15
			return image.Rect(x, y-13, x+7, y)
		}
	}
	return image.Rectangle{}
}

func obscureEndingBalance(img *image.Gray, body []string) {
	bounds := endingDigitBounds(body)
	original := append([]byte(nil), img.Pix...)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			total, n := 0, 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					p := image.Pt(x+dx, y+dy)
					if p.In(img.Bounds()) {
						total += int(original[p.Y*img.Stride+p.X])
						n++
					}
				}
			}
			shade := total / n
			if x < bounds.Min.X+3 {
				shade = (shade + 225) / 2
			}
			img.SetGray(x, y, color.Gray{Y: uint8(shade)})
		}
	}
}

func noisyScanPDF(path string, d docSpec) error {
	body := lines(d)
	const w = 520
	h := 18 + (len(body)+2)*15
	img := image.NewGray(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.Gray{Y: 225}}, image.Point{}, draw.Src)
	drawer := &font.Drawer{Dst: img, Src: &image.Uniform{color.Gray{Y: 70}}, Face: basicfont.Face7x13}
	for i, l := range append(body, "", footer) {
		drawer.Dot = fixed.P(12, 18+i*15)
		drawer.DrawString(l)
	}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < w*h/9; i++ {
		img.SetGray(rng.Intn(w), rng.Intn(h), color.Gray{Y: uint8(90 + rng.Intn(140))})
	}
	obscureEndingBalance(img, body)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	pdf := fpdf.New("P", "mm", "Letter", "")
	if err := keywords(pdf, d); err != nil {
		return err
	}
	pdf.AddPage()
	pdf.RegisterImageOptionsReader("scan", fpdf.ImageOptions{ImageType: "PNG"}, &buf)
	pdf.ImageOptions("scan", 10, 10, 195, 0, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	return pdf.OutputFileAndClose(path)
}
func writeDoc(path string, d docSpec) error {
	if d.LowQ {
		return noisyScanPDF(path, d)
	}
	return textPDF(path, d)
}

func generate(out string) error {
	for _, sc := range demoSets {
		dir := filepath.Join(out, "documents", sc.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for _, d := range sc.Docs {
			if err := writeDoc(filepath.Join(dir, d.Name), d); err != nil {
				return fmt.Errorf("%s/%s: %w", sc.ID, d.Name, err)
			}
		}
	}
	for _, sc := range scenarios {
		dir := filepath.Join(out, "documents", sc.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		fx := fixture{ID: sc.ID, Description: sc.Description, Expected: sc.Expected}
		for _, d := range sc.Docs {
			path := filepath.Join(dir, d.Name)
			if err := writeDoc(path, d); err != nil {
				return fmt.Errorf("%s/%s: %w", sc.ID, d.Name, err)
			}
			fx.Documents = append(fx.Documents, fixtureDoc{File: filepath.Join("documents", sc.ID, d.Name), ExpectedDocType: d.Type, ExpectedFields: d.Fields})
		}
		sort.Slice(fx.Documents, func(i, j int) bool { return fx.Documents[i].File < fx.Documents[j].File })
		if err := os.MkdirAll(filepath.Join(out, "fixtures"), 0o755); err != nil {
			return err
		}
		raw, err := json.MarshalIndent(fx, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, "fixtures", sc.ID+".json"), raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}
func main() {
	out := flag.String("out", filepath.Join("..", "testdata"), "output directory")
	flag.Parse()
	if err := generate(*out); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %d scenarios to %s", len(scenarios), *out)
}
