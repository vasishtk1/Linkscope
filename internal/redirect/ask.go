package redirect

import (
	"context"
	"fmt"
	"strings"

	"github.com/vasishtk1/Linkscope/internal/linkcheck"
	"github.com/vasishtk1/Linkscope/internal/pagemeta"
	"github.com/vasishtk1/Linkscope/internal/trace"
)

const MaxChoices = 10

// Question is what the model is shown: the lost page, what it used to be, and the closest live pages.
type Question struct {
	Page       string          `json:"page"`
	Old        pagemeta.Info   `json:"old"`
	Candidates []pagemeta.Info `json:"candidates"`
}

// Answer is the model's pick: a candidate URL or "" for none, with how sure it is and why.
type Answer struct {
	Choice     string `json:"choice,omitempty"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
	Thinking   string `json:"thinking,omitempty"`
	Model      string `json:"model,omitempty"`
}

// Picker chooses a redirect target from a Question; the Gemini client does this.
type Picker interface {
	Pick(ctx context.Context, q Question) (Answer, error)
}

// Describer reads what pages are about; pagemeta.Reader does this.
type Describer interface {
	Page(ctx context.Context, rawURL string) pagemeta.Info
	Archived(ctx context.Context, page string) pagemeta.Info
}

// Ask has the model decide a page the address matching left unclear, and fills in s if it picks a candidate that loads.
func Ask(ctx context.Context, p Picker, d Describer, v Verifier, s Suggestion, live []string, log *trace.Log) Suggestion {
	ranked := Rank(s.Page, live)
	if len(ranked) == 0 {
		log.Say("decide", s.Page, "no live page shares a word with it, so there is nothing to choose from")
		return s
	}
	ranked = ranked[:min(MaxChoices, len(ranked))]

	q := Question{Page: s.Page}
	q.Old = d.Archived(ctx, s.Page)
	if q.Old.Title != "" {
		log.Say("archive", s.Page, "the Internet Archive's copy from %s was titled %q", q.Old.Archived, q.Old.Title)
	} else {
		log.Say("archive", s.Page, "no readable copy in the Internet Archive, deciding from the address alone")
	}
	log.Say("read", s.Page, "reading the titles of the %d closest live pages", len(ranked))
	for _, c := range ranked {
		q.Candidates = append(q.Candidates, d.Page(ctx, c.URL))
	}
	log.Say("ask", s.Page, "asking Gemini which page replaces it, if any")

	a, err := p.Pick(ctx, q)
	if err != nil {
		log.Say("ask", s.Page, "Gemini failed: %v", err)
		s.NoMatch += "; Gemini failed"
		return s
	}
	a.Choice = matchChoice(a.Choice, q.Candidates)
	s.AI = &a
	switch {
	case a.Choice == "":
		log.Add(trace.Event{Step: "decide", Page: s.Page, Text: "none of them fit: " + a.Reason, Thinking: a.Thinking})
		s.NoMatch = "Gemini: none fit"
		return s
	case a.Confidence != "high":
		log.Add(trace.Event{Step: "decide", Page: s.Page, Text: fmt.Sprintf("leaning to %s but only %s confidence, so leaving it: %s", a.Choice, a.Confidence, a.Reason), Thinking: a.Thinking})
		s.NoMatch = "Gemini unsure"
		return s
	}
	log.Add(trace.Event{Step: "decide", Page: s.Page, Text: fmt.Sprintf("picked %s (%s): %s", a.Choice, a.Model, a.Reason), Thinking: a.Thinking})
	chk := v.Check(ctx, bare(a.Choice))
	if chk.Result != linkcheck.OK {
		log.Say("verify", s.Page, "%s didn't load (%s), so no redirect", a.Choice, chk.Result)
		s.NoMatch = "Gemini's pick didn't load"
		return s
	}
	s.Target = a.Choice
	if chk.FinalURL != "" {
		s.Target = chk.FinalURL
	}
	log.Say("verify", s.Page, "%s loads, redirect it there", s.Target)
	s.NoMatch, s.By = "", "gemini"
	for i := range ranked {
		if ranked[i].URL == a.Choice {
			s.Pick = &ranked[i]
		}
	}
	return s
}

// matchChoice keeps the model's pick only if it is one of the candidates, so it can never invent an address.
func matchChoice(choice string, cands []pagemeta.Info) string {
	choice = strings.TrimSpace(choice)
	for _, c := range cands {
		if choice != "" && c.URL == choice {
			return c.URL
		}
	}
	return ""
}
