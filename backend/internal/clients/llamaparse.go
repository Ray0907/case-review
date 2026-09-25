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

func (l *LlamaParse) Parse(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filepath.Base(path))
	if _, err := io.Copy(fw, f); err != nil {
		return "", err
	}
	mw.WriteField("configuration", `{"tier":"agentic","version":"latest"}`)
	mw.Close()
	req, _ := http.NewRequestWithContext(ctx, "POST", l.baseURL+"/api/v2/parse/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	var created struct {
		ID string `json:"id"`
	}
	if err := l.do(req, &created); err != nil {
		return "", err
	}
	for {
		req, _ := http.NewRequestWithContext(ctx, "GET", l.baseURL+"/api/v2/parse/"+created.ID, nil)
		var st struct {
			Job struct {
				Status string `json:"status"`
			} `json:"job"`
		}
		if err := l.do(req, &st); err != nil {
			return "", err
		}
		switch st.Job.Status {
		case "COMPLETED":
			req, _ := http.NewRequestWithContext(ctx, "GET", l.baseURL+"/api/v2/parse/"+created.ID+"?expand=markdown_full", nil)
			var out struct {
				MarkdownFull string `json:"markdown_full"`
			}
			if err := l.do(req, &out); err != nil {
				return "", err
			}
			return out.MarkdownFull, nil
		case "FAILED", "CANCELLED":
			return "", fmt.Errorf("llamaparse job %s %s", created.ID, st.Job.Status)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(l.pollEvery):
		}
	}
}
