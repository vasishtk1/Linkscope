package redirect

import (
	"context"

	"github.com/vasishtk1/Linkscope/internal/linkcheck"
)

const maxVerify = 3

// Suggestion is the redirect target picked for one broken page, or why none was picked.
type Suggestion struct {
	Page     string     `json:"page"`
	Target   string     `json:"target,omitempty"`
	Pick     *Candidate `json:"pick,omitempty"`
	RunnerUp *Candidate `json:"runnerUp,omitempty"`
	NoMatch  string     `json:"noMatch,omitempty"`
}

// Verifier checks that a picked page loads; linkcheck.Checker does this.
type Verifier interface {
	Check(ctx context.Context, page string) linkcheck.Check
}

// Suggest picks a target for a broken page: the clear winner by address that loads, trying at most maxVerify picks.
func Suggest(ctx context.Context, v Verifier, page string, live []string) Suggestion {
	s := Suggestion{Page: page}
	ranked := Rank(page, live)
	for tries := 0; tries < maxVerify; tries++ {
		if !Clear(ranked) {
			break
		}
		best := ranked[0]
		chk := v.Check(ctx, bare(best.URL))
		if chk.Result == linkcheck.OK {
			s.Target = best.URL
			if chk.FinalURL != "" {
				s.Target = chk.FinalURL
			}
			s.Pick = &best
			if len(ranked) > 1 {
				s.RunnerUp = &ranked[1]
			}
			return s
		}
		ranked = ranked[1:]
	}
	switch {
	case len(ranked) == 0:
		s.NoMatch = "no live page shares a word with it"
	case Clear(ranked):
		s.NoMatch = "the best matches didn't load"
	case ranked[0].Score < MinScore:
		s.NoMatch = "closest page is too different"
		s.Pick = &ranked[0]
	default:
		s.NoMatch = "top two are too close to call"
		s.Pick = &ranked[0]
		s.RunnerUp = &ranked[1]
	}
	return s
}

func bare(rawURL string) string {
	for _, p := range []string{"https://", "http://"} {
		if len(rawURL) > len(p) && rawURL[:len(p)] == p {
			return rawURL[len(p):]
		}
	}
	return rawURL
}
