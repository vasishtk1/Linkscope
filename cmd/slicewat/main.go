package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/vasishtk1/Linkscope/internal/warc"
)

var responseKey = []byte(`"HTTP-Response-Metadata"`)

func main() {
	in := flag.String("in", "data/sample.warc.wat.gz", "WAT file to cut up")
	out := flag.String("out", "data/demo/files", "folder for the smaller files")
	files := flag.Int("files", 10, "how many files to make")
	pages := flag.Int("pages", 2000, "pages per file")
	flag.Parse()
	if *files < 1 || *pages < 1 {
		fmt.Fprintln(os.Stderr, "slicewat: -files and -pages must be at least 1")
		os.Exit(2)
	}
	if err := run(*in, *out, *files, *pages); err != nil {
		fmt.Fprintln(os.Stderr, "slicewat:", err)
		os.Exit(1)
	}
}

// run copies consecutive records into small files, starting a new one every `pages` pages, and prints each file's name.
func run(in, out string, files, pages int) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	rd := warc.NewReader(zr)
	for i := 1; i <= files; i++ {
		name := fmt.Sprintf("demo-%03d.warc.wat.gz", i)
		n, err := writeFile(rd, filepath.Join(out, name), pages)
		if err != nil {
			return err
		}
		if n == 0 {
			os.Remove(filepath.Join(out, name))
			return fmt.Errorf("%s ran out of pages after %d files", in, i-1)
		}
		fmt.Println(name)
		if n < pages {
			fmt.Fprintf(os.Stderr, "slicewat: %s ran out after %d files; the last has %d pages\n", in, i, n)
			return nil
		}
	}
	return nil
}

// writeFile copies records until it has written `pages` HTTP responses or the input ends, and returns how many it wrote.
func writeFile(rd *warc.Reader, path string, pages int) (int, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	w := bufio.NewWriter(f)
	n := 0
	for n < pages {
		rec, err := rd.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			f.Close()
			return 0, err
		}
		if rec.Type != "metadata" {
			continue
		}
		if err := warc.WriteRecord(w, rec); err != nil {
			f.Close()
			return 0, err
		}
		if bytes.Contains(rec.Body, responseKey) {
			n++
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return 0, err
	}
	return n, f.Close()
}
