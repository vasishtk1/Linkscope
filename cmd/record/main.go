package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vasishtk1/Linkscope/internal/jobs"
	"github.com/vasishtk1/Linkscope/internal/record"
)

func main() {
	db := flag.String("db", "data/demo/jobs.db", "job database of the run to save")
	out := flag.String("o", "data/run.json", "where to write the recording")
	name := flag.String("name", "", "label shown on the website, e.g. \"20 Common Crawl files, 4 workers\"")
	flag.Parse()
	if err := run(*db, *out, *name); err != nil {
		fmt.Fprintln(os.Stderr, "record:", err)
		os.Exit(1)
	}
}

func run(dbPath, out, name string) error {
	if _, err := os.Stat(dbPath); err != nil {
		return err
	}
	s, err := jobs.Open(dbPath, 1)
	if err != nil {
		return err
	}
	defer s.Close()
	r, err := record.FromStore(s, name, time.Now())
	if err != nil {
		return err
	}
	if r.Name == "" {
		r.Name = fmt.Sprintf("%d files, %d workers", r.Totals.Files, len(r.Workers))
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	tmp := out + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, out); err != nil {
		return err
	}
	fmt.Printf("saved %q to %s (%d KB): %d files (%d done, %d failed), %d events, %d workers, %.1fs\n",
		r.Name, out, len(b)/1000, r.Totals.Files, r.Totals.Done, r.Totals.Failed, len(r.Events), len(r.Workers), r.Totals.Seconds)
	if r.Derived {
		fmt.Println("note: this database has no event log, so the history was rebuilt from each finished file's timing; retries aren't shown")
	}
	return nil
}
