package redirect

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vasishtk1/Linkscope/internal/linkcheck"
	"github.com/vasishtk1/Linkscope/internal/pagemeta"
	"github.com/vasishtk1/Linkscope/internal/trace"
)

type fakePicker struct {
	a   Answer
	err error
	got Question
}

func (f *fakePicker) Pick(_ context.Context, q Question) (Answer, error) {
	f.got = q
	return f.a, f.err
}

type fakeDescriber struct{}

func (fakeDescriber) Page(_ context.Context, u string) pagemeta.Info {
	return pagemeta.Info{URL: u, Title: "title of " + u}
}

func (fakeDescriber) Archived(_ context.Context, page string) pagemeta.Info {
	return pagemeta.Info{URL: "https://" + page, Title: "Jobs at Example", Archived: "20230819"}
}

func TestAsk(t *testing.T) {
	live := []string{
		"https://example.com/careers-openings",
		"https://example.com/news/coaching-openings-2026",
		"https://example.com/write-for-us",
		"https://example.com/broken-openings",
	}
	unclear := Suggestion{Page: "example.com/openings", NoMatch: "closest page is too different"}
	v := fakeVerifier{"example.com/broken-openings": linkcheck.Broken}

	cases := []struct {
		name                string
		a                   Answer
		err                 error
		target, noMatch, by string
	}{
		{"high confidence pick", Answer{Choice: "https://example.com/careers-openings", Confidence: "high", Reason: "same jobs page"}, nil, "https://example.com/careers-openings", "", "gemini"},
		{"none fit", Answer{Confidence: "high", Reason: "nothing is a jobs page"}, nil, "", "Gemini: none fit", ""},
		{"not sure", Answer{Choice: "https://example.com/careers-openings", Confidence: "medium", Reason: "maybe"}, nil, "", "Gemini unsure", ""},
		{"invented address", Answer{Choice: "https://example.com/jobs", Confidence: "high", Reason: "made up"}, nil, "", "Gemini: none fit", ""},
		{"pick doesn't load", Answer{Choice: "https://example.com/broken-openings", Confidence: "high", Reason: "x"}, nil, "", "Gemini's pick didn't load", ""},
		{"request fails", Answer{}, errors.New("error 429"), "", "closest page is too different; Gemini failed", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &fakePicker{a: c.a, err: c.err}
			log := trace.New(nil)
			s := Ask(context.Background(), p, fakeDescriber{}, v, unclear, live, log)
			if s.Target != c.target || s.NoMatch != c.noMatch || s.By != c.by {
				t.Errorf("got target %q noMatch %q by %q, want %q %q %q", s.Target, s.NoMatch, s.By, c.target, c.noMatch, c.by)
			}
			if len(p.got.Candidates) != 3 || p.got.Old.Title != "Jobs at Example" || !strings.HasPrefix(p.got.Candidates[0].Title, "title of ") {
				t.Errorf("question = %+v", p.got)
			}
			if len(log.Events()) < 3 {
				t.Errorf("only %d steps logged", len(log.Events()))
			}
		})
	}
}

func TestAskNothingToChooseFrom(t *testing.T) {
	p := &fakePicker{}
	s := Ask(context.Background(), p, fakeDescriber{}, fakeVerifier{}, Suggestion{Page: "example.com/zzz", NoMatch: "x"}, []string{"https://example.com/abc"}, nil)
	if s.NoMatch != "x" || p.got.Page != "" {
		t.Errorf("asked anyway: %+v", s)
	}
}

func TestAskCapsCandidates(t *testing.T) {
	var live []string
	for _, w := range strings.Fields("a b c d e f g h i j k l") {
		live = append(live, "https://example.com/shoes-"+w)
	}
	p := &fakePicker{a: Answer{Confidence: "low"}}
	Ask(context.Background(), p, fakeDescriber{}, fakeVerifier{}, Suggestion{Page: "example.com/shoes"}, live, nil)
	if len(p.got.Candidates) != MaxChoices {
		t.Errorf("%d candidates, want %d", len(p.got.Candidates), MaxChoices)
	}
}
