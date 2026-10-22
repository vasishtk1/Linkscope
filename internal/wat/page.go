package wat

import (
	"encoding/json"
	"strconv"
)

type Link struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	Rel  string `json:"rel"`
}

type Page struct {
	URL    string
	Status int
	Base   string
	Links  []Link
}

type metadata struct {
	Envelope struct {
		PayloadMetadata struct {
			HTTPResponseMetadata *struct {
				ResponseMessage struct {
					Status string `json:"Status"`
				} `json:"Response-Message"`
				HTMLMetadata struct {
					Head struct {
						Base string `json:"Base"`
					} `json:"Head"`
					Links []Link `json:"Links"`
				} `json:"HTML-Metadata"`
			} `json:"HTTP-Response-Metadata"`
		} `json:"Payload-Metadata"`
	} `json:"Envelope"`
}

// ParsePage returns ok=false for records that aren't HTTP responses.
func ParsePage(targetURI string, body []byte) (Page, bool, error) {
	var m metadata
	if err := json.Unmarshal(body, &m); err != nil {
		return Page{}, false, err
	}
	resp := m.Envelope.PayloadMetadata.HTTPResponseMetadata
	if resp == nil {
		return Page{}, false, nil
	}
	status, _ := strconv.Atoi(resp.ResponseMessage.Status)
	return Page{
		URL:    targetURI,
		Status: status,
		Base:   resp.HTMLMetadata.Head.Base,
		Links:  resp.HTMLMetadata.Links,
	}, true, nil
}
