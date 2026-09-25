package main

import (
	"bytes"
	"strings"
	"testing"

	"tidalwave/backend/internal/evalscore"
)

func TestPrintCalibration(t *testing.T) {
	var out bytes.Buffer
	clean, flag, high := .76, .3, .91
	printCalibration(&out, map[string]evalscore.CalibrationRow{
		"document_authenticity": {Threshold: .5, Clean: evalscore.ScoreRange{Min: &clean, Max: &clean, Count: 1}, Flag: evalscore.ScoreRange{Min: &flag, Max: &high, Count: 2}},
		"ocr_quality":           {Threshold: .8, Clean: evalscore.ScoreRange{Min: &clean, Max: &clean, Count: 1}, Flag: evalscore.ScoreRange{Min: &flag, Max: &high, Count: 2}, FalsePositiveRisk: true},
	})
	text := out.String()
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], "flag min") || strings.Contains(lines[0], "flag min/max") ||
		!strings.Contains(lines[1], "document_authenticity") || strings.Contains(lines[1], "false-positive risk") ||
		!strings.Contains(lines[2], "ocr_quality") || !strings.Contains(lines[2], "false-positive risk") ||
		!strings.Contains(lines[1], "0.30") || strings.Contains(lines[1], "0.30 (2)") || strings.Contains(text, "0.91") || strings.Contains(text, "overlap") {
		t.Fatalf("table: %s", text)
	}
}
