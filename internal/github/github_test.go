package github_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vasishtk1/Linkscope/internal/github"
	"github.com/vasishtk1/Linkscope/internal/github/githubtest"
)

func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestLoadKey(t *testing.T) {
	k := newKey(t)
	dir := t.TempDir()
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(k)
	files := map[string][]byte{
		"pkcs1.pem": pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}),
		"pkcs8.pem": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}),
		"bad.pem":   []byte("not a key"),
	}
	for name, data := range files {
		os.WriteFile(filepath.Join(dir, name), data, 0o600)
	}
	for _, name := range []string{"pkcs1.pem", "pkcs8.pem"} {
		got, err := github.LoadKey(filepath.Join(dir, name))
		if err != nil || !got.Equal(k) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := github.LoadKey(filepath.Join(dir, "bad.pem")); err == nil {
		t.Error("bad.pem: want an error")
	}
}

func TestRepoFlow(t *testing.T) {
	k := newKey(t)
	fake := githubtest.New(t, &k.PublicKey, "123", "me", "site", map[string]string{"vercel.json": "{}\n"})
	app := &github.App{ID: "123", Key: k, BaseURL: fake.URL}
	ctx := context.Background()

	repo, err := app.Repo(ctx, "me", "site")
	if err != nil {
		t.Fatal(err)
	}
	base, err := repo.DefaultBranch(ctx)
	if err != nil || base != "main" {
		t.Fatalf("DefaultBranch = %q, %v", base, err)
	}
	content, sha, err := repo.File(ctx, "vercel.json", base)
	if err != nil || string(content) != "{}\n" || sha != githubtest.BlobSHA("{}\n") {
		t.Fatalf("File = %q, %q, %v", content, sha, err)
	}
	if _, _, err := repo.File(ctx, "netlify.toml", base); !errors.Is(err, github.ErrNotFound) {
		t.Errorf("missing file: %v", err)
	}
	head, _ := repo.BranchSHA(ctx, base)
	if err := repo.CreateBranch(ctx, "fix", head); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateBranch(ctx, "fix", head); !errors.Is(err, github.ErrExists) {
		t.Errorf("second CreateBranch: %v", err)
	}
	if err := repo.PutFile(ctx, "vercel.json", "fix", "msg", []byte(`{"a":1}`), sha); err != nil {
		t.Fatal(err)
	}
	if err := repo.PutFile(ctx, "vercel.json", "fix", "msg", []byte(`{"a":2}`), sha); err == nil {
		t.Error("stale sha: want an error")
	}
	pr, err := repo.CreatePR(ctx, "Title", "Body", "fix", base)
	if err != nil || pr.Number != 1 || !strings.HasSuffix(pr.URL, "/pull/1") {
		t.Fatalf("CreatePR = %+v, %v", pr, err)
	}
	open, err := repo.OpenPRs(ctx)
	if err != nil || len(open) != 1 || open[0].Head.Ref != "fix" {
		t.Errorf("OpenPRs = %+v, %v", open, err)
	}
	if fake.Branches["main"]["vercel.json"] != "{}\n" {
		t.Error("main branch was changed")
	}
}

func TestErrors(t *testing.T) {
	k := newKey(t)
	fake := githubtest.New(t, &k.PublicKey, "123", "me", "site", nil)
	ctx := context.Background()

	if _, err := (&github.App{ID: "123", Key: k, BaseURL: fake.URL}).Repo(ctx, "me", "other"); err == nil || !strings.Contains(err.Error(), "isn't installed") {
		t.Errorf("not installed: %v", err)
	}
	if _, err := (&github.App{ID: "123", Key: newKey(t), BaseURL: fake.URL}).Repo(ctx, "me", "site"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("wrong key: %v", err)
	}
	old := func() time.Time { return time.Now().Add(-time.Hour) }
	if _, err := (&github.App{ID: "123", Key: k, BaseURL: fake.URL, Now: old}).Repo(ctx, "me", "site"); err == nil {
		t.Error("expired token: want an error")
	}
	repo, err := (&github.App{ID: "123", Key: k, BaseURL: fake.URL}).Repo(ctx, "me", "site")
	if err != nil {
		t.Fatal(err)
	}
	err = repo.PutFile(ctx, "x", "nope", "m", nil, "")
	if err == nil || strings.Contains(err.Error(), githubtest.Token) {
		t.Errorf("error must exist and never include the token: %v", err)
	}
}
