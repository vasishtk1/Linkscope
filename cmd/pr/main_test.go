package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vasishtk1/Linkscope/internal/github/githubtest"
)

const checksJSON = `{"site":"fansided.com","pages":[
 {"page":"fansided.com/","sites":5},
 {"page":"fansided.com/fansided-openings","sites":5,"suggestion":{"page":"fansided.com/fansided-openings","noMatch":"Gemini: none fit","ai":{"confidence":"high","reason":"None of the candidates are about jobs | contributors."}}},
 {"page":"fansided.com/mobile","sites":1,"suggestion":{"page":"fansided.com/mobile","target":"https://fansided.com/fansided-mobile-app","pick":{"url":"https://fansided.com/fansided-mobile-app","path":"/fansided-mobile-app","score":0.5,"shared":["mobile"]},"by":"address"}}
]}`

// setup starts a fake GitHub with the given files and writes the key, .env and checks the command reads.
func setup(t *testing.T, files map[string]string) (*githubtest.Fake, config) {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "app.pem")
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}), 0o600)
	env := filepath.Join(dir, ".env")
	os.WriteFile(env, []byte("GITHUB_APP_ID=123\nGITHUB_APP_KEY="+keyPath+"\n"), 0o600)
	in := filepath.Join(dir, "checks.json")
	os.WriteFile(in, []byte(checksJSON), 0o600)
	fake := githubtest.New(t, &k.PublicKey, "123", "me", "site", files)
	day := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	return fake, config{in: in, repo: "me/site", env: env, baseURL: fake.URL, now: func() time.Time { return day }}
}

func runPR(t *testing.T, cfg config) string {
	t.Helper()
	var out bytes.Buffer
	if err := run(context.Background(), &out, cfg, nil); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	return out.String()
}

func TestPreviewChangesNothing(t *testing.T) {
	fake, cfg := setup(t, map[string]string{"vercel.json": "{\n  \"cleanUrls\": true\n}\n"})
	out := runPR(t, cfg)
	for _, want := range []string{
		"Title: Redirect 1 broken page that other sites still link to",
		"| `/mobile` | 1 | `/fansided-mobile-app` | address match | shares \"mobile\" (score 0.50) |",
		"- `/fansided-openings`: Gemini: None of the candidates are about jobs | contributors.",
		`"source": "/mobile"`,
		"Run again with -open",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("preview is missing %q\n%s", want, out)
		}
	}
	if len(fake.Writes) != 0 {
		t.Errorf("preview wrote to GitHub: %v", fake.Writes)
	}
}

func TestOpen(t *testing.T) {
	fake, cfg := setup(t, map[string]string{"vercel.json": "{\n  \"cleanUrls\": true\n}\n"})
	cfg.open = true
	out := runPR(t, cfg)
	if !strings.Contains(out, "Opened pull request #1: https://github.com/me/site/pull/1") {
		t.Errorf("output:\n%s", out)
	}
	want := []string{"branch linkscope/redirects-2026-11-17", "put linkscope/redirects-2026-11-17 vercel.json", "pr linkscope/redirects-2026-11-17"}
	if strings.Join(fake.Writes, ";") != strings.Join(want, ";") {
		t.Errorf("writes = %v", fake.Writes)
	}
	changed := fake.Branches["linkscope/redirects-2026-11-17"]["vercel.json"]
	if !strings.Contains(changed, `"destination": "/fansided-mobile-app"`) || !strings.Contains(changed, `"cleanUrls": true`) {
		t.Errorf("new vercel.json:\n%s", changed)
	}
	if fake.Branches["main"]["vercel.json"] != "{\n  \"cleanUrls\": true\n}\n" {
		t.Error("main was changed")
	}

	// A second run doesn't open a duplicate while the first is open.
	out = runPR(t, cfg)
	if !strings.Contains(out, "already open: #1") || len(fake.Writes) != 3 {
		t.Errorf("duplicate run:\n%s\nwrites %v", out, fake.Writes)
	}

	// Once it's closed, a new run on the same day gets a new branch name.
	fake.PRs = nil
	runPR(t, cfg)
	if _, ok := fake.Branches["linkscope/redirects-2026-11-17-2"]; !ok {
		t.Errorf("no -2 branch: writes %v", fake.Writes)
	}
}

func TestNoRedirectFile(t *testing.T) {
	_, cfg := setup(t, map[string]string{"index.html": "<h1>hi</h1>"})
	var out bytes.Buffer
	if err := run(context.Background(), &out, cfg, nil); err == nil || !strings.Contains(err.Error(), "-create") {
		t.Errorf("want a -create hint, got %v", err)
	}
	fake, cfg := setup(t, map[string]string{"index.html": "<h1>hi</h1>"})
	cfg.create, cfg.open = "netlify", true
	runPR(t, cfg)
	if got := fake.Branches["linkscope/redirects-2026-11-17"]["public/_redirects"]; !strings.Contains(got, "/mobile /fansided-mobile-app 301") {
		t.Errorf("new _redirects:\n%s", got)
	}
}

func TestBadInput(t *testing.T) {
	_, cfg := setup(t, nil)
	cfg.repo = "no-slash"
	if err := run(context.Background(), &bytes.Buffer{}, cfg, nil); err == nil {
		t.Error("bad -repo: want an error")
	}
	_, cfg = setup(t, nil)
	cfg.env = filepath.Join(t.TempDir(), "missing.env")
	if err := run(context.Background(), &bytes.Buffer{}, cfg, nil); err == nil || !strings.Contains(err.Error(), "GITHUB_APP_ID") {
		t.Errorf("missing settings: %v", err)
	}
}
