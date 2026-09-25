package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"tidalwave/backend/internal/auth"
	"tidalwave/backend/internal/clients"
	"tidalwave/backend/internal/config"
	"tidalwave/backend/internal/httpapi"
	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/pipeline/fake"
	"tidalwave/backend/internal/sse"
	"tidalwave/backend/internal/store"
)

func main() {
	cfg := config.Load()
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "uploads"), 0o755); err != nil {
		log.Fatal(err)
	}
	s, err := store.Open(filepath.Join(cfg.DataDir, "harbor.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	hash, err := auth.HashPassword(cfg.SeedPassword)
	if err != nil {
		log.Fatal(err)
	}
	if err := s.EnsureUser(cfg.SeedEmail, "Maya Park", "Senior Underwriter", hash); err != nil {
		log.Fatal(err)
	}
	broker := sse.New()
	var stages pipeline.Stages
	switch cfg.PipelineMode {
	case "fake":
		stages = fake.Stages()
		log.Print("pipeline: fake stages (no external calls)")
	default:
		var err error
		if stages, err = clients.RealStages(cfg); err != nil {
			log.Fatal(err)
		}
		if cfg.AnthropicBaseURL != "" {
			log.Printf("claude traffic via %s", cfg.AnthropicBaseURL)
		}
	}
	runner := pipeline.NewRunner(s, stages, broker)
	handler := httpapi.New(s, httpapi.Options{StaticDir: cfg.StaticDir, UploadDir: filepath.Join(cfg.DataDir, "uploads"), Runner: runner, Broker: broker}).Handler()
	log.Printf("listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, handler))
}
