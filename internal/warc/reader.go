package warc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Record struct {
	Type      string
	TargetURI string
	Body      []byte
}

type Reader struct {
	r *bufio.Reader
}

func NewReader(r io.Reader) *Reader {
	return &Reader{r: bufio.NewReaderSize(r, 1<<20)}
}

// Next returns io.EOF once there are no more records.
func (rd *Reader) Next() (*Record, error) {
	version, err := rd.readNonEmptyLine()
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(version, "WARC/") {
		return nil, fmt.Errorf("warc: expected version line, got %q", version)
	}

	rec := &Record{}
	length := -1
	for {
		line, err := rd.readLine()
		if err != nil {
			return nil, unexpected(err)
		}
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(name) {
		case "warc-type":
			rec.Type = value
		case "warc-target-uri":
			rec.TargetURI = value
		case "content-length":
			length, err = strconv.Atoi(value)
			if err != nil || length < 0 {
				return nil, fmt.Errorf("warc: bad Content-Length %q", value)
			}
		}
	}
	if length < 0 {
		return nil, errors.New("warc: record missing Content-Length")
	}

	rec.Body = make([]byte, length)
	if _, err := io.ReadFull(rd.r, rec.Body); err != nil {
		return nil, unexpected(err)
	}
	return rec, nil
}

func (rd *Reader) readLine() (string, error) {
	line, err := rd.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// Skips the blank lines that separate records.
func (rd *Reader) readNonEmptyLine() (string, error) {
	for {
		line, err := rd.readLine()
		if err != nil {
			return "", err
		}
		if line != "" {
			return line, nil
		}
	}
}

func unexpected(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}
