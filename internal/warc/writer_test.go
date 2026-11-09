package warc

import (
	"bytes"
	"compress/gzip"
	"io"
	"reflect"
	"testing"
)

func TestWriteRecordRoundTrip(t *testing.T) {
	in := []*Record{
		{Type: "metadata", TargetURI: "https://a.com/", Body: []byte(`{"x":1}`)},
		{Type: "metadata", TargetURI: "https://b.com/page", Body: []byte("line one\r\nline two")},
	}
	var buf bytes.Buffer
	for _, r := range in {
		if err := WriteRecord(&buf, r); err != nil {
			t.Fatal(err)
		}
	}
	zr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	rd := NewReader(zr)
	for i, want := range in {
		got, err := rd.Next()
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("record %d = %+v, want %+v", i, got, want)
		}
	}
	if _, err := rd.Next(); err != io.EOF {
		t.Fatalf("after last record: %v, want EOF", err)
	}
}
