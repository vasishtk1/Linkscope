package linkcheck

import "strings"

// Robots holds the robots.txt rules that apply to our bot on one host.
type Robots struct {
	rules    []rule
	Sitemaps []string
}

type rule struct {
	allow   bool
	pattern string
}

var allowAll = &Robots{}

// ParseRobots keeps the rules for our bot's group, or the "*" group when the file has none for us.
func ParseRobots(text, token string) *Robots {
	token = strings.ToLower(token)
	var mine, star []rule
	var sitemaps []string
	foundMine := false
	var agents []string
	inRules := false
	for _, line := range strings.Split(text, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "sitemap":
			if val != "" {
				sitemaps = append(sitemaps, val)
			}
		case "user-agent":
			if inRules {
				agents, inRules = nil, false
			}
			a := strings.ToLower(val)
			agents = append(agents, a)
			if a == token || strings.HasPrefix(a, token+"/") {
				foundMine = true
			}
		case "allow", "disallow":
			inRules = true
			if val == "" {
				continue
			}
			r := rule{allow: key == "allow", pattern: val}
			for _, a := range agents {
				switch {
				case a == token || strings.HasPrefix(a, token+"/"):
					mine = append(mine, r)
				case a == "*":
					star = append(star, r)
				}
			}
		}
	}
	if foundMine {
		return &Robots{rules: mine, Sitemaps: sitemaps}
	}
	return &Robots{rules: star, Sitemaps: sitemaps}
}

// Allowed applies the longest matching rule; Allow wins a tie, and no match means allowed.
func (r *Robots) Allowed(path string) bool {
	best, allow := -1, true
	for _, ru := range r.rules {
		if !match(ru.pattern, path) {
			continue
		}
		if n := len(ru.pattern); n > best || (n == best && ru.allow) {
			best, allow = n, ru.allow
		}
	}
	return allow
}

// match supports robots.txt's two special characters: * for any text and $ for the end of the path.
func match(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	parts := strings.Split(strings.TrimSuffix(pattern, "$"), "*")
	if !strings.HasPrefix(path, parts[0]) {
		return false
	}
	rest := path[len(parts[0]):]
	for i, p := range parts[1:] {
		if anchored && i == len(parts)-2 {
			return strings.HasSuffix(rest, p)
		}
		j := strings.Index(rest, p)
		if j < 0 {
			return false
		}
		rest = rest[j+len(p):]
	}
	return !anchored || rest == ""
}
