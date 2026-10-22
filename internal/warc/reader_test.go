package warc

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
)

func record(typ, uri, body string) string {
	return "WARC/1.0\r\n" +
		"WARC-Type: " + typ + "\r\n" +
		"WARC-Target-URI: " + uri + "\r\n" +
		"Content-Length: " + strconv.Itoa(len(body)) + "\r\n" +
		"\r\n" + body + "\r\n\r\n"
}

func TestReadsRecordsInOrder(t *testing.T) {
	input := record("warcinfo", "", "info") + record("metadata", "http://a.com/", `{"x":1}`)
	rd := NewReader(strings.NewReader(input))

	first, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	if first.Type != "warcinfo" || string(first.Body) != "info" {
		t.Fatalf("first record = %+v", first)
	}
	second, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	if second.Type != "metadata" || second.TargetURI != "http://a.com/" || string(second.Body) != `{"x":1}` {
		t.Fatalf("second record = %+v", second)
	}
	if _, err := rd.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestBodyMayContainBlankLines(t *testing.T) {
	body := "line1\r\n\r\nWARC/1.0\r\nline3"
	rd := NewReader(strings.NewReader(record("metadata", "http://a.com/", body)))
	rec, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	if string(rec.Body) != body {
		t.Fatalf("body = %q", rec.Body)
	}
}

func TestTruncatedBody(t *testing.T) {
	input := "WARC/1.0\r\nWARC-Type: metadata\r\nContent-Length: 100\r\n\r\nshort"
	_, err := NewReader(strings.NewReader(input)).Next()
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestMissingContentLength(t *testing.T) {
	input := "WARC/1.0\r\nWARC-Type: metadata\r\n\r\nbody"
	if _, err := NewReader(strings.NewReader(input)).Next(); err == nil {
		t.Fatal("expected error")
	}
}

func TestGarbageInsteadOfVersion(t *testing.T) {
	if _, err := NewReader(strings.NewReader("hello\r\n")).Next(); err == nil {
		t.Fatal("expected error")
	}
}
