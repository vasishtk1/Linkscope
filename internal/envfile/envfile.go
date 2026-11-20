package envfile

import (
	"bufio"
	"os"
	"strings"
)

// Get returns a setting from the environment, or else from a .env file at path; "" if neither has it.
func Get(path, name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), name+"="); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}
