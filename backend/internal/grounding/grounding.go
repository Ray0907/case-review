// Package grounding maps extracted values to unambiguous source evidence.
package grounding

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Box struct {
	Page       int     `json:"page"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	W          float64 `json:"w"`
	H          float64 `json:"h"`
	PageWidth  float64 `json:"page_width"`
	PageHeight float64 `json:"page_height"`
}

type Token struct {
	Text  string
	Box   Box
	Group int // Consecutive words in one item, or a single table cell.
}

type rect struct{ X, Y, W, H float64 }
type word struct {
	Span []int
	BBox rect
}
type line struct{ Words []word }
type cell struct {
	Span []int
	BBox []rect
}
type page struct {
	Number  int     `json:"page_number"`
	Width   float64 `json:"page_width"`
	Height  float64 `json:"page_height"`
	Success bool
	Items   []struct {
		MD        string
		Grounding struct {
			Lines []line
			Rows  [][]*cell
		}
	}
}

// Decode reads the JSONL sidecar. Failed pages and misaligned tables are not evidence.
func Decode(r io.Reader) ([]Token, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var tokens []Token
	group := 0
	for scanner.Scan() {
		if len(strings.TrimSpace(scanner.Text())) == 0 {
			continue
		}
		var p page
		if err := json.Unmarshal(scanner.Bytes(), &p); err != nil {
			return nil, err
		}
		if !p.Success || p.Number < 1 || p.Width <= 0 || p.Height <= 0 {
			continue
		}
		boxFor := func(b rect) (Box, bool) {
			box := Box{p.Number, b.X, b.Y, b.W, b.H, p.Width, p.Height}
			return box, b.X >= 0 && b.Y >= 0 && b.W > 0 && b.H > 0 && b.X+b.W <= p.Width && b.Y+b.H <= p.Height
		}
		for _, item := range p.Items {
			group++
			if len(item.Grounding.Rows) == 0 {
				for _, l := range item.Grounding.Lines {
					for _, w := range l.Words {
						text, ok := spanText(item.MD, w.Span)
						box, valid := boxFor(w.BBox)
						if !ok || !valid {
							group++
							continue
						}
						tokens = append(tokens, Token{text, box, group})
					}
				}
				continue
			}
			rows := tableRows(item.MD)
			aligned := len(rows) == len(item.Grounding.Rows)
			for i, row := range item.Grounding.Rows {
				if i >= len(rows) || len(row) != len(rows[i]) {
					aligned = false
					break
				}
			}
			if !aligned {
				continue
			}
			for i, row := range item.Grounding.Rows {
				for j, c := range row {
					group++
					if c == nil || len(c.BBox) == 0 {
						continue
					}
					text, ok := spanText(strings.TrimSpace(rows[i][j]), c.Span)
					if !ok {
						continue
					}
					var box Box
					valid := true
					for k, b := range c.BBox {
						part, ok := boxFor(b)
						if !ok {
							valid = false
							break
						}
						if k == 0 {
							box = part
						} else {
							box = union(box, part)
						}
					}
					if valid {
						tokens = append(tokens, Token{text, box, group})
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return tokens, nil
}

func spanText(text string, span []int) (string, bool) {
	if len(span) != 2 || span[0] < 0 || span[1] <= span[0] || span[1] > len(text) {
		return "", false
	}
	part := text[span[0]:span[1]]
	return part, utf8.ValidString(part)
}

func tableRows(md string) [][]string {
	var rows [][]string
	for _, ln := range strings.Split(md, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var cells []string
		start := 0
		escaped := false
		for i, c := range ln {
			if c == '|' && !escaped {
				cells = append(cells, ln[start:i])
				start = i + 1
			}
			if c == '\\' {
				escaped = !escaped
			} else {
				escaped = false
			}
		}
		cells = append(cells, ln[start:])
		if strings.HasPrefix(ln, "|") {
			cells = cells[1:]
		}
		if strings.HasSuffix(ln, "|") && len(cells) > 0 && cells[len(cells)-1] == "" {
			cells = cells[:len(cells)-1]
		}
		separator := len(cells) > 0
		for _, c := range cells {
			s := strings.Trim(strings.TrimSpace(c), ":")
			if len(s) < 3 || strings.Trim(s, "-") != "" {
				separator = false
			}
		}
		if !separator {
			rows = append(rows, cells)
		}
	}
	return rows
}

func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.NewReplacer("$", "", ",", "").Replace(s))), " ")
}

func union(a, b Box) Box {
	right, bottom := math.Max(a.X+a.W, b.X+b.W), math.Max(a.Y+a.H, b.Y+b.H)
	a.X, a.Y = math.Min(a.X, b.X), math.Min(a.Y, b.Y)
	a.W, a.H = right-a.X, bottom-a.Y
	return a
}

// Match trusts exactly one occurrence, never picks the first of several hits.
func Match(tokens []Token, value any) *Box {
	if value == nil {
		return nil
	}
	target := normalize(fmt.Sprint(value))
	if target == "" {
		return nil
	}
	number, numErr := strconv.ParseFloat(target, 64)
	var found *Box
	hits := 0
	for start := 0; start < len(tokens); {
		end := start + 1
		for end < len(tokens) && tokens[end].Group == tokens[start].Group && tokens[end].Box.Page == tokens[start].Box.Page {
			end++
		}
		group := tokens[start:end]
		if numErr == nil {
			for _, t := range group {
				for _, part := range strings.Fields(normalize(t.Text)) {
					n, err := strconv.ParseFloat(part, 64)
					if err == nil && n == number {
						b := t.Box
						found = &b
						hits++
					}
				}
			}
		} else {
			var parts []string
			var offsets []int
			length := 0
			for _, t := range group {
				s := normalize(t.Text)
				offsets = append(offsets, length)
				parts = append(parts, s)
				length += len(s) + 1
			}
			text := strings.Join(parts, " ")
			for from := 0; from < len(text); {
				i := strings.Index(text[from:], target)
				if i < 0 {
					break
				}
				i += from
				j := i + len(target)
				from = i + 1
				if !boundary(text, i, true) || !boundary(text, j, false) {
					continue
				}
				var b *Box
				for k, t := range group {
					if offsets[k] < j && offsets[k]+len(parts[k]) > i {
						if b == nil {
							copy := t.Box
							b = &copy
						} else {
							*b = union(*b, t.Box)
						}
					}
				}
				if b != nil {
					found = b
					hits++
				}
			}
		}
		if hits > 1 {
			return nil
		}
		start = end
	}
	if hits == 1 {
		return found
	}
	return nil
}

func boundary(s string, i int, before bool) bool {
	if i == 0 || i == len(s) {
		return true
	}
	var r rune
	if before {
		r, _ = utf8.DecodeLastRuneInString(s[:i])
	} else {
		r, _ = utf8.DecodeRuneInString(s[i:])
	}
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}
