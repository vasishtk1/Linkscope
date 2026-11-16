package gemini

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/vasishtk1/Linkscope/internal/pagemeta"
	"github.com/vasishtk1/Linkscope/internal/redirect"
)

func TestPrompt(t *testing.T) {
	q := redirect.Question{
		Page: "example.com/jobs",
		Old:  pagemeta.Info{Title: "Jobs at Example", Archived: "20230819"},
		Candidates: []pagemeta.Info{
			{URL: "https://example.com/careers", Title: "Careers"},
			{URL: "https://example.com/news/jobs-report"},
		},
	}
	p := Prompt(q)
	for _, want := range []string{"Lost page: https://example.com/jobs", "from 20230819", "title: Jobs at Example", "1. https://example.com/careers", "title: Careers", "2. https://example.com/news/jobs-report", "title: (none)"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q:\n%s", want, p)
		}
	}
	if p := Prompt(redirect.Question{Page: "example.com/x"}); !strings.Contains(p, "No archived copy") {
		t.Errorf("no archive note:\n%s", p)
	}
}

func TestParse(t *testing.T) {
	a, err := Parse(`{"choice":" https://example.com/careers ","confidence":"high","reason":"same page"}`, " thinking \n")
	if err != nil || a.Choice != "https://example.com/careers" || a.Confidence != "high" || a.Thinking != "thinking" {
		t.Errorf("Parse = %+v, %v", a, err)
	}
	for _, bad := range []string{``, `not json`, `{"choice":"x","confidence":"very","reason":"r"}`} {
		if _, err := Parse(bad, ""); err == nil {
			t.Errorf("Parse(%q) had no error", bad)
		}
	}
}

func TestRetryable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{genai.APIError{Code: 429, Message: "quota"}, true},
		{genai.APIError{Code: 503, Message: "high demand"}, true},
		{genai.APIError{Code: 504}, true},
		{genai.APIError{Code: 404}, true},
		{genai.APIError{Code: 404, Message: "model not found"}, false},
		{genai.APIError{Code: 400, Message: "bad request"}, false},
		{context.DeadlineExceeded, true},
		{errors.New("other"), false},
	}
	for _, c := range cases {
		if got := retryable(c.err); got != c.want {
			t.Errorf("retryable(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestShort(t *testing.T) {
	err := short(genai.APIError{Code: 503, Message: "This model is busy. Please try again later."})
	if err.Error() != "error 503: This model is busy" {
		t.Errorf("short = %q", err)
	}
}

func TestKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("OTHER=1\nGEMINI_API_KEY=\"abc\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GEMINI_API_KEY", "")
	if k := Key(path); k != "abc" {
		t.Errorf("Key from file = %q", k)
	}
	t.Setenv("GEMINI_API_KEY", "fromenv")
	if k := Key(path); k != "fromenv" {
		t.Errorf("Key from env = %q", k)
	}
	t.Setenv("GEMINI_API_KEY", "")
	if k := Key(filepath.Join(t.TempDir(), "missing")); k != "" {
		t.Errorf("Key with no file = %q", k)
	}
}
