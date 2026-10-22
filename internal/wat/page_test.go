package wat

import (
	"reflect"
	"testing"
)

const responseRecord = `{"Envelope":{"Payload-Metadata":{"Actual-Content-Type":"application/http; msgtype=response",
"HTTP-Response-Metadata":{"Response-Message":{"Status":"200","Version":"HTTP/1.1","Reason":"OK"},
"HTML-Metadata":{"Head":{"Title":"Pans","Base":"https://pinchofyum.com/"},
"Links":[{"path":"A@/href","url":"https://ourplace.com/","text":"pan"},{"path":"A@/href","url":"/about","rel":"nofollow"}]}}}}}`

const requestRecord = `{"Envelope":{"Payload-Metadata":{"Actual-Content-Type":"application/http; msgtype=request",
"HTTP-Request-Metadata":{"Request-Message":{"Method":"GET","Path":"/"}}}}}`

func TestParseResponse(t *testing.T) {
	p, ok, err := ParsePage("https://pinchofyum.com/best-pans", []byte(responseRecord))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	want := Page{
		URL:    "https://pinchofyum.com/best-pans",
		Status: 200,
		Base:   "https://pinchofyum.com/",
		Links: []Link{
			{Path: "A@/href", URL: "https://ourplace.com/"},
			{Path: "A@/href", URL: "/about", Rel: "nofollow"},
		},
	}
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("got %+v\nwant %+v", p, want)
	}
}

func TestParseIgnoresNonResponses(t *testing.T) {
	_, ok, err := ParsePage("https://a.com/", []byte(requestRecord))
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want false and nil", ok, err)
	}
}

func TestParseRejectsInvalidJSON(t *testing.T) {
	if _, _, err := ParsePage("https://a.com/", []byte(`{"Envelope":`)); err == nil {
		t.Fatal("expected error")
	}
}
