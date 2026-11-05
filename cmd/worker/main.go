package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/vasishtk1/Linkscope/internal/worker"
)

func main() {
	host, _ := os.Hostname()
	id := flag.String("id", fmt.Sprintf("%s-%d", host, os.Getpid()), "worker name shown in the coordinator")
	coord := flag.String("coordinator", "http://localhost:8080", "coordinator URL")
	source := flag.String("source", "https://data.commoncrawl.org", "Common Crawl base URL")
	out := flag.String("out", "data/results", "directory for result files")
	parsers := flag.Int("parsers", runtime.NumCPU(), "parser goroutines per file")
	retries := flag.Int("fetch-retries", 6, "download retries before failing a job")
	busy := flag.Duration("busy-backoff", 15*time.Second, "first wait when Common Crawl says it's busy; doubles up to 2m")
	reqTimeout := flag.Duration("request-timeout", 20*time.Second, "time limit for each call to the coordinator")
	firstByte := flag.Duration("first-byte-timeout", 30*time.Second, "how long Common Crawl can take to start sending a file")
	stall := flag.Duration("stall-timeout", 20*time.Second, "cancel a download when no data arrives for this long")
	flag.Parse()
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.SetPrefix(*id + " ")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w := worker.New(worker.Config{
		ID:           *id,
		Coordinator:  *coord,
		Source:       *source,
		OutDir:       *out,
		Parsers:      *parsers,
		FetchRetries: *retries,
		RetryBackoff: time.Second,
		BusyBackoff:  *busy,

		RequestTimeout:   *reqTimeout,
		FirstByteTimeout: *firstByte,
		StallTimeout:     *stall,
	})
	if err := w.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
