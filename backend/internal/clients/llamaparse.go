package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"tidalwave/backend/internal/grounding"
)

type LlamaParse struct {
	baseURL, apiKey string
	http            *http.Client
	pollEvery       time.Duration
}

func NewLlamaParse(baseURL, apiKey string) *LlamaParse {
	return &LlamaParse{baseURL: baseURL, apiKey: apiKey, http: &http.Client{Timeout: 60 * time.Second}, pollEvery: 2 * time.Second}
}

func (l *LlamaParse) do(req *http.Request, out any) error {
	req.Header.Set("Authorization", "Bearer "+l.apiKey)
	req.Header.Set("Accept", "application/json")
	res, err := l.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("llamaparse %s %s: %d", req.Method, req.URL.Path, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// Grounding is optional: a missing, unavailable or malformed sidecar must not
// prevent extraction. Never attach the API key to a presigned storage request.
func (l *LlamaParse) tokenBoxes(ctx context.Context, url string) []grounding.Token {
	if url == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}
	res, err := l.http.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil
	}
	tokens, err := grounding.Decode(res.Body)
	if err != nil {
		return nil
	}
	return tokens
}

func (l *LlamaParse) Parse(ctx context.Context, path string) (string, []grounding.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filepath.Base(path))
	if _, err := io.Copy(fw, f); err != nil {
		return "", nil, err
	}
	mw.WriteField("configuration", `{"tier":"agentic","version":"latest","output_options":{"granular_bboxes":["word","line","cell"]}}`)
	mw.Close()
	req, _ := http.NewRequestWithContext(ctx, "POST", l.baseURL+"/api/v2/parse/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	var created struct {
		ID string `json:"id"`
	}
	if err := l.do(req, &created); err != nil {
		return "", nil, err
	}
	for {
		req, _ := http.NewRequestWithContext(ctx, "GET", l.baseURL+"/api/v2/parse/"+created.ID, nil)
		var st struct {
			Job struct {
				Status string `json:"status"`
			} `json:"job"`
		}
		if err := l.do(req, &st); err != nil {
			return "", nil, err
		}
		switch st.Job.Status {
		case "COMPLETED":
			req, _ := http.NewRequestWithContext(ctx, "GET", l.baseURL+"/api/v2/parse/"+created.ID+"?expand=markdown_full", nil)
			var out struct {
				MarkdownFull string `json:"markdown_full"`
				Metadata     struct {
					GroundedItems struct {
						URL string `json:"presigned_url"`
					} `json:"grounded_items"`
				} `json:"result_content_metadata"`
			}
			if err := l.do(req, &out); err != nil {
				return "", nil, err
			}
			return out.MarkdownFull, l.tokenBoxes(ctx, out.Metadata.GroundedItems.URL), nil
		case "FAILED", "CANCELLED":
			return "", nil, fmt.Errorf("llamaparse job %s %s", created.ID, st.Job.Status)
		}
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-time.After(l.pollEvery):
		}
	}
}
