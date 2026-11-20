package main

import (
	"fmt"
	"strings"

	"github.com/vasishtk1/Linkscope/internal/redirectfile"
)

func prTitle(n int) string {
	if n == 1 {
		return "Redirect 1 broken page that other sites still link to"
	}
	return fmt.Sprintf("Redirect %d broken pages that other sites still link to", n)
}

// prBody writes the pull request description: what was added and why, what was skipped, and pages left alone.
func prBody(c checks, res redirectfile.Result, fixed map[string]page, file string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Linkscope found pages on %s that are broken but still get links from other websites. This adds permanent redirects to `%s` so those visitors land on a live page instead.\n\n", c.Site, file)
	b.WriteString("| Broken page | Linking sites | Redirects to | Picked by | Why |\n|---|---|---|---|---|\n")
	for _, r := range res.Added {
		p := fixed[r.From]
		by, why := reason(p)
		fmt.Fprintf(&b, "| `%s` | %d | `%s` | %s | %s |\n", cell(r.From), p.Sites, cell(r.To), by, cell(why))
	}
	if len(res.Skipped) > 0 {
		b.WriteString("\n**Skipped:**\n")
		for _, s := range res.Skipped {
			fmt.Fprintf(&b, "- `%s` → `%s`: %s\n", s.From, s.To, s.Reason)
		}
	}
	var left []string
	for _, p := range c.Pages {
		if p.Suggestion != nil && p.Suggestion.Target == "" {
			why := p.Suggestion.NoMatch
			if p.Suggestion.AI != nil && p.Suggestion.AI.Reason != "" {
				why = "Gemini: " + p.Suggestion.AI.Reason
			}
			left = append(left, fmt.Sprintf("- `%s`: %s", redirectfile.New(p.Page, "/").From, why))
		}
	}
	if len(left) > 0 {
		b.WriteString("\n**Not redirected** (no live page is a good replacement):\n")
		b.WriteString(strings.Join(left, "\n") + "\n")
	}
	b.WriteString("\nPlease check each target before merging. Linkscope never merges on its own.\n")
	return b.String()
}

// reason says who picked a page's target and why, from its suggestion.
func reason(p page) (by, why string) {
	s := p.Suggestion
	if s == nil {
		return "", ""
	}
	if s.By == "gemini" && s.AI != nil {
		return "Gemini", s.AI.Reason
	}
	if s.Pick != nil && len(s.Pick.Shared) > 0 {
		return "address match", fmt.Sprintf("shares %q (score %.2f)", strings.Join(s.Pick.Shared, ", "), s.Pick.Score)
	}
	return "address match", ""
}

// cell keeps a value from breaking the Markdown table.
func cell(s string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ", "`", "'").Replace(s)
}
