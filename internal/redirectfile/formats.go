package redirectfile

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const header = "# Broken pages that other sites still link to"

type member struct {
	key string
	val json.RawMessage
}

type vercelRedirect struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Permanent   bool   `json:"permanent"`
}

// parseObject reads a JSON object's members in file order, so rewriting it doesn't shuffle the keys.
func parseObject(content []byte) ([]member, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(content))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("vercel.json is not a JSON object")
	}
	var out []member
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("vercel.json: %w", err)
		}
		var m member
		m.key, _ = t.(string)
		if err := dec.Decode(&m.val); err != nil {
			return nil, fmt.Errorf("vercel.json: %w", err)
		}
		out = append(out, m)
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("vercel.json: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("vercel.json has extra text after the object")
	}
	return out, nil
}

func vercelSources(content []byte) ([]string, error) {
	members, err := parseObject(content)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range members {
		switch m.key {
		case "routes":
			return nil, errors.New("vercel.json uses the old \"routes\" setting, which can't be combined with \"redirects\"")
		case "redirects":
			var rs []vercelRedirect
			if err := json.Unmarshal(m.val, &rs); err != nil {
				return nil, fmt.Errorf("vercel.json redirects: %w", err)
			}
			for _, r := range rs {
				out = append(out, r.Source)
			}
		}
	}
	return out, nil
}

// addVercel puts the new redirects first in "redirects", keeping every other key and its order.
func addVercel(content []byte, rs []Redirect) ([]byte, error) {
	members, err := parseObject(content)
	if err != nil {
		return nil, err
	}
	var items []json.RawMessage
	for _, r := range rs {
		b, err := marshal(vercelRedirect{Source: r.From, Destination: r.To, Permanent: true})
		if err != nil {
			return nil, err
		}
		items = append(items, b)
	}
	found := false
	for i, m := range members {
		if m.key != "redirects" {
			continue
		}
		var old []json.RawMessage
		if err := json.Unmarshal(m.val, &old); err != nil {
			return nil, err
		}
		if members[i].val, err = marshal(append(items, old...)); err != nil {
			return nil, err
		}
		found = true
	}
	if !found {
		b, err := marshal(items)
		if err != nil {
			return nil, err
		}
		members = append(members, member{"redirects", b})
	}
	var flat bytes.Buffer
	flat.WriteByte('{')
	for i, m := range members {
		if i > 0 {
			flat.WriteByte(',')
		}
		k, err := marshal(m.key)
		if err != nil {
			return nil, err
		}
		flat.Write(k)
		flat.WriteByte(':')
		flat.Write(m.val)
	}
	flat.WriteByte('}')
	var out bytes.Buffer
	if err := json.Indent(&out, flat.Bytes(), "", indentOf(content)); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// marshal is json.Marshal without turning & < > into \u escapes, which would make URLs hard to read.
func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// indentOf is the indent of the file's first indented line, so the rewritten file keeps its style; two spaces if none.
func indentOf(content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed != "" && len(trimmed) < len(line) {
			return line[:len(line)-len(trimmed)]
		}
	}
	return "  "
}

func netlifySources(content []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(content))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) > 0 && !strings.HasPrefix(f[0], "#") {
			out = append(out, f[0])
		}
	}
	return out
}

// addNetlify puts the new rules at the top: Netlify uses the first rule that matches, so a catch-all lower down can't hide them.
func addNetlify(content []byte, rs []Redirect) []byte {
	var b bytes.Buffer
	b.WriteString(header + "\n")
	for _, r := range rs {
		fmt.Fprintf(&b, "%s %s 301\n", r.From, r.To)
	}
	if len(bytes.TrimSpace(content)) > 0 {
		b.WriteByte('\n')
		b.Write(content)
		if !bytes.HasSuffix(content, []byte("\n")) {
			b.WriteByte('\n')
		}
	}
	return b.Bytes()
}

var tomlFrom = regexp.MustCompile(`^\s*from\s*=\s*["']([^"']*)["']`)

func tomlSources(content []byte) []string {
	var out []string
	for _, line := range strings.Split(string(content), "\n") {
		if m := tomlFrom.FindStringSubmatch(line); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

// addTOML puts the new [[redirects]] blocks before the first existing one, for the same first-match reason, or at the end if there are none.
func addTOML(content []byte, rs []Redirect) []byte {
	var block strings.Builder
	block.WriteString(header + "\n")
	for _, r := range rs {
		fmt.Fprintf(&block, "[[redirects]]\n  from = %q\n  to = %q\n  status = 301\n\n", r.From, r.To)
	}
	text := string(content)
	lines := strings.SplitAfter(text, "\n")
	pos := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "[[redirects]]" {
			pos = i
			break
		}
	}
	if pos >= 0 {
		return []byte(strings.Join(lines[:pos], "") + block.String() + strings.Join(lines[pos:], ""))
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if strings.TrimSpace(text) != "" {
		text += "\n"
	}
	return []byte(text + strings.TrimSuffix(block.String(), "\n"))
}
