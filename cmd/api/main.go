package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vasishtk1/Linkscope/internal/index"
	"github.com/vasishtk1/Linkscope/internal/record"
	"github.com/vasishtk1/Linkscope/internal/serve"
)

func main() {
	addr := flag.String("addr", ":8081", "listen address")
	dir := flag.String("index", "data/index", "index folder from cmd/index")
	runPath := flag.String("run", "data/run.json", "saved run from cmd/record for the pipeline replay (optional)")
	flag.Parse()
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	if err := run(*addr, *dir, *runPath); err != nil {
		log.Fatal(err)
	}
}

func run(addr, dir, runPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	start := time.Now()
	ix, err := index.Open(dir)
	if err != nil {
		return err
	}
	defer ix.Close()
	m := ix.Manifest()
	log.Printf("opened %s: %d domains, %d edges, %d shards in %s", dir, m.Domains, m.Edges, m.Shards, time.Since(start).Round(time.Millisecond))

	saved, err := loadRun(runPath)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           serve.Handler(ix, saved),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Printf("api listening on %s", addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// loadRun reads a saved run; a missing file just means the website has nothing to replay.
func loadRun(path string) (*record.Recording, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		log.Printf("no saved run at %s; /run will answer 404", path)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r record.Recording
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	log.Printf("loaded saved run %q from %s: %d files, %d events", r.Name, path, r.Totals.Files, len(r.Events))
	return &r, nil
}
