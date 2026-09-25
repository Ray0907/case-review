package clients

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tidalwave/backend/internal/config"
	"tidalwave/backend/internal/schemas"
)

func syntheticW2PDF() []byte {
	stream := "BT /F1 14 Tf 72 720 Td (IRS Form W-2 Wage and Tax Statement - SYNTHETIC) Tj 0 -22 Td (Employer: Northwind Logistics) Tj 0 -22 Td (Tax year: 2025) Tj 0 -22 Td (Box 1 Wages: 86400.00) Tj 0 -22 Td (Box 2 Federal tax withheld: 11230.00) Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes()
}

func TestLiveLlamaParseAndJev(t *testing.T) {
	if os.Getenv("HARBOR_LIVE") != "1" {
		t.Skip("opt-in live integration test")
	}
	cfg := config.Load()
	if cfg.LlamaParseKey == "" || cfg.TypeSafeKey == "" {
		t.Fatal("live integration requires LLAMAPARSE_API_KEY and TYPESAFE_API_KEY (aliases accepted)")
	}
	path := filepath.Join(t.TempDir(), "synthetic-w2.pdf")
	if err := os.WriteFile(path, syntheticW2PDF(), 0o600); err != nil {
		t.Fatal("could not create synthetic PDF")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	text, err := NewLlamaParse(cfg.LlamaParseURL, cfg.LlamaParseKey).Parse(ctx, path)
	if err != nil {
		if strings.Contains(err.Error(), "llamaparse POST /api/v2/parse/upload: 404") {
			t.Fatal("LlamaParse live smoke: HTTP 404 POST /api/v2/parse/upload")
		}
		t.Fatal("LlamaParse live smoke failed (response details suppressed)")
	}
	if strings.TrimSpace(text) == "" {
		t.Fatal("LlamaParse returned empty markdown")
	}
	choice, _, err := NewJev(cfg.TypeSafeKey, "").Classify(ctx, text)
	if err != nil {
		t.Fatal("Jev live classification failed (response details suppressed)")
	}
	for _, known := range append(schemas.Types, schemas.Other) {
		if choice == known {
			return
		}
	}
	t.Fatal("Jev returned a choice outside the six supported types")
}
