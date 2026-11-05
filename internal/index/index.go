package index

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Backlink struct {
	Domain        string `json:"domain"`
	Pages         int    `json:"pages"`
	NofollowPages int    `json:"nofollowPages"`
	ExampleSource string `json:"exampleSource"`
	ExampleTarget string `json:"exampleTarget"`
}

type Entry struct {
	Domain         string
	Score          int
	LinkingDomains int
	LinkingPages   int
	NofollowPages  int
	shard          int
	offset         int64
	length         int64
}

type shard struct {
	f       *os.File
	entries []Entry
}

type Index struct {
	m      Manifest
	shards []shard
}

// Open reads the manifest and every shard's directory into memory; backlink lists stay on disk.
func Open(dir string) (*Index, error) {
	b, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", manifestName, err)
	}
	if m.Shards < 1 || m.Hash != "fnv1a-64" {
		return nil, fmt.Errorf("%s: unsupported index (shards %d, hash %q)", manifestName, m.Shards, m.Hash)
	}
	ix := &Index{m: m}
	for i := range m.Shards {
		s, err := openShard(filepath.Join(dir, shardName(i)), i)
		if err != nil {
			ix.Close()
			return nil, fmt.Errorf("%s: %w", shardName(i), err)
		}
		ix.shards = append(ix.shards, s)
	}
	return ix, nil
}

func openShard(path string, n int) (shard, error) {
	f, err := os.Open(path)
	if err != nil {
		return shard{}, err
	}
	entries, err := readDirectory(f, n)
	if err != nil {
		f.Close()
		return shard{}, err
	}
	return shard{f: f, entries: entries}, nil
}

func readDirectory(f *os.File, n int) ([]Entry, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	if size < footerSize {
		return nil, errors.New("file too small")
	}
	var footer [footerSize]byte
	if _, err := f.ReadAt(footer[:], size-footerSize); err != nil {
		return nil, err
	}
	if string(footer[8:]) != magic {
		return nil, errors.New("not an index shard (bad footer)")
	}
	dirOffset := int64(binary.BigEndian.Uint64(footer[:8]))
	if dirOffset > size-footerSize {
		return nil, errors.New("directory offset past end of file")
	}

	var entries []Entry
	sc := bufio.NewScanner(io.NewSectionReader(f, dirOffset, size-footerSize-dirOffset))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for line := 1; sc.Scan(); line++ {
		e, err := parseEntry(sc.Text(), n, dirOffset)
		if err != nil {
			return nil, fmt.Errorf("directory line %d: %w", line, err)
		}
		if len(entries) > 0 && entries[len(entries)-1].Domain >= e.Domain {
			return nil, fmt.Errorf("directory line %d: domains not sorted", line)
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

func parseEntry(line string, n int, dataEnd int64) (Entry, error) {
	f := strings.Split(line, "\t")
	if len(f) != 7 || f[0] == "" {
		return Entry{}, fmt.Errorf("want 7 fields, got %d", len(f))
	}
	var nums [6]int64
	for i := range nums {
		v, err := strconv.ParseInt(f[i+1], 10, 64)
		if err != nil {
			return Entry{}, err
		}
		nums[i] = v
	}
	e := Entry{
		Domain: f[0], Score: int(nums[0]), LinkingDomains: int(nums[1]), LinkingPages: int(nums[2]),
		NofollowPages: int(nums[3]), shard: n, offset: nums[4], length: nums[5],
	}
	if e.offset < 0 || e.length < 0 || e.offset+e.length > dataEnd {
		return Entry{}, errors.New("backlink range outside the data section")
	}
	return e, nil
}

func (ix *Index) Close() error {
	var errs []error
	for _, s := range ix.shards {
		errs = append(errs, s.f.Close())
	}
	return errors.Join(errs...)
}

func (ix *Index) Manifest() Manifest { return ix.m }

// Find hashes the domain to its shard, then binary-searches that shard's sorted directory.
func (ix *Index) Find(domain string) (Entry, bool) {
	entries := ix.shards[ShardOf(domain, ix.m.Shards)].entries
	i := sort.Search(len(entries), func(i int) bool { return entries[i].Domain >= domain })
	if i < len(entries) && entries[i].Domain == domain {
		return entries[i], true
	}
	return Entry{}, false
}

// Backlinks reads a domain's list from disk with one read; limit 0 means all of them.
func (ix *Index) Backlinks(e Entry, limit int) ([]Backlink, error) {
	if e.length == 0 {
		return []Backlink{}, nil
	}
	buf := make([]byte, e.length)
	if _, err := ix.shards[e.shard].f.ReadAt(buf, e.offset); err != nil {
		return nil, err
	}
	n := e.LinkingDomains
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]Backlink, 0, n)
	rest := string(buf)
	for len(out) < n {
		line, after, ok := strings.Cut(rest, "\n")
		if !ok {
			return nil, fmt.Errorf("%s: backlink list cut short", e.Domain)
		}
		b, err := parseBacklink(line)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Domain, err)
		}
		out = append(out, b)
		rest = after
	}
	return out, nil
}

func parseBacklink(line string) (Backlink, error) {
	f := strings.Split(line, "\t")
	if len(f) != 5 {
		return Backlink{}, fmt.Errorf("want 5 fields, got %d", len(f))
	}
	pages, err1 := strconv.Atoi(f[1])
	nofollow, err2 := strconv.Atoi(f[2])
	if err := errors.Join(err1, err2); err != nil {
		return Backlink{}, err
	}
	return Backlink{Domain: f[0], Pages: pages, NofollowPages: nofollow, ExampleSource: f[3], ExampleTarget: f[4]}, nil
}
