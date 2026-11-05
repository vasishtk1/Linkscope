package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vasishtk1/Linkscope/internal/index"
	"github.com/vasishtk1/Linkscope/internal/serve"
)

func main() {
	addr := flag.String("addr", ":8081", "listen address")
	dir := flag.String("index", "data/index", "index folder from cmd/index")
	flag.Parse()
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	if err := run(*addr, *dir); err != nil {
		log.Fatal(err)
	}
}

func run(addr, dir string) error {
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

	srv := &http.Server{
		Addr:              addr,
		Handler:           serve.Handler(ix),
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
