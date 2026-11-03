package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/vasishtk1/Linkscope/internal/coordinator"
	"github.com/vasishtk1/Linkscope/internal/jobs"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "data/jobs.db", "SQLite job database")
	source := flag.String("source", "https://data.commoncrawl.org", "Common Crawl base URL")
	crawl := flag.String("crawl", "CC-MAIN-2026-43", "crawl to process")
	pathsFrom := flag.String("paths", "", "WAT path list (URL or file); defaults to the crawl's wat.paths.gz")
	offset := flag.Int("offset", 0, "skip this many files at the start of the list")
	limit := flag.Int("limit", 20, "number of files to queue (0 = all)")
	lease := flag.Duration("lease", 60*time.Second, "how long a worker holds a job without a heartbeat")
	maxAttempts := flag.Int("max-attempts", 3, "attempts before a job is marked failed")
	flag.Parse()
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	if err := run(*addr, *dbPath, *source, *crawl, *pathsFrom, *offset, *limit, *lease, *maxAttempts); err != nil {
		log.Fatal(err)
	}
}

func run(addr, dbPath, source, crawl, pathsFrom string, offset, limit int, lease time.Duration, maxAttempts int) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return err
	}
	store, err := jobs.Open(dbPath, maxAttempts)
	if err != nil {
		return err
	}
	defer store.Close()

	if pathsFrom == "" {
		pathsFrom = coordinator.CrawlPathsURL(source, crawl)
	}
	all, err := coordinator.LoadPaths(ctx, http.DefaultClient, pathsFrom)
	if err != nil {
		return err
	}
	added, err := store.Add(coordinator.Window(all, offset, limit))
	if err != nil {
		return err
	}
	log.Printf("%d files in list, %d new jobs queued", len(all), added)

	srv := coordinator.New(store, lease)
	if err := srv.MarkStarted(); err != nil {
		return err
	}
	go srv.RunReaper(ctx, time.Second)

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdown)
	}()
	log.Printf("coordinator listening on %s (lease %s, max %d attempts)", addr, lease, maxAttempts)
	if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
