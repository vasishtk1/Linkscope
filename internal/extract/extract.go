package extract

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/vasishtk1/Linkscope/internal/links"
	"github.com/vasishtk1/Linkscope/internal/warc"
	"github.com/vasishtk1/Linkscope/internal/wat"
)

type Stats struct {
	Records     int
	Responses   int
	PagesUsed   int
	CrossLinks  int
	ParseErrors int
}

type outcome uint8

const (
	parseError outcome = iota
	notResponse
	skipped
	used
)

type result struct {
	outcome  outcome
	outlinks links.Outlinks
}

// Run streams a gzipped WAT file through a pool of parser goroutines and builds the link graph.
func Run(r io.Reader, workers int) (*links.Graph, Stats, error) {
	var s Stats
	if workers < 1 {
		workers = 1
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, s, err
	}
	defer gz.Close()

	records := make(chan *warc.Record, 256)
	results := make(chan result, 256)

	var readErr error
	var recordCount int
	go func() {
		defer close(records)
		rd := warc.NewReader(gz)
		for {
			rec, err := rd.Next()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				readErr = fmt.Errorf("record %d: %w", recordCount+1, err)
				return
			}
			recordCount++
			if rec.Type == "metadata" {
				records <- rec
			}
		}
	}()

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for rec := range records {
				results <- parse(rec)
			}
		})
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	g := links.NewGraph()
	for res := range results {
		switch res.outcome {
		case parseError:
			s.ParseErrors++
		case skipped:
			s.Responses++
		case used:
			s.Responses++
			s.PagesUsed++
			s.CrossLinks += len(res.outlinks.Targets)
			g.Add(res.outlinks)
		}
	}
	if readErr != nil {
		return nil, s, readErr
	}
	s.Records = recordCount
	return g, s, nil
}

func parse(rec *warc.Record) result {
	page, ok, err := wat.ParsePage(rec.TargetURI, rec.Body)
	if err != nil {
		return result{outcome: parseError}
	}
	if !ok {
		return result{outcome: notResponse}
	}
	o, ok := links.Extract(page)
	if !ok {
		return result{outcome: skipped}
	}
	return result{outcome: used, outlinks: o}
}
