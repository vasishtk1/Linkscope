package githubtest

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const Token = "ghs_fake_installation_token"

// Fake is a tiny in-memory GitHub for tests: one app, one installed repo, branches of files, and pull requests.
type Fake struct {
	URL      string
	AppID    string
	Owner    string
	Name     string
	Default  string
	Branches map[string]map[string]string
	PRs      []map[string]any
	Writes   []string

	key *rsa.PublicKey
	mu  sync.Mutex
}

// New starts a fake GitHub where the app is installed on owner/name, with files on the main branch.
func New(t *testing.T, key *rsa.PublicKey, appID, owner, name string, files map[string]string) *Fake {
	f := &Fake{AppID: appID, Owner: owner, Name: name, Default: "main", Branches: map[string]map[string]string{"main": files}, key: key}
	mux := http.NewServeMux()
	repo := "/repos/" + owner + "/" + name
	mux.HandleFunc("GET "+repo+"/installation", f.app(func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]any{"id": 42}) }))
	mux.HandleFunc("POST /app/installations/42/access_tokens", f.app(func(w http.ResponseWriter, r *http.Request) {
		reply(w, 201, map[string]any{"token": Token})
	}))
	mux.HandleFunc("GET "+repo, f.repo(func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"default_branch": f.Default})
	}))
	mux.HandleFunc("GET "+repo+"/contents/{path...}", f.repo(f.getFile))
	mux.HandleFunc("PUT "+repo+"/contents/{path...}", f.repo(f.putFile))
	mux.HandleFunc("GET "+repo+"/git/ref/heads/{branch...}", f.repo(f.getRef))
	mux.HandleFunc("POST "+repo+"/git/refs", f.repo(f.createRef))
	mux.HandleFunc("GET "+repo+"/pulls", f.repo(func(w http.ResponseWriter, r *http.Request) { reply(w, 200, f.PRs) }))
	mux.HandleFunc("POST "+repo+"/pulls", f.repo(f.createPR))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

// BlobSHA is the fake's stand-in for a file's blob SHA.
func BlobSHA(content string) string {
	s := sha1.Sum([]byte(content))
	return hex.EncodeToString(s[:])
}

func (f *Fake) app(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := f.checkJWT(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); err != nil {
			reply(w, 401, map[string]any{"message": err.Error()})
			return
		}
		f.lock(h)(w, r)
	}
}

func (f *Fake) repo(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+Token {
			reply(w, 401, map[string]any{"message": "Bad credentials"})
			return
		}
		f.lock(h)(w, r)
	}
}

func (f *Fake) lock(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		h(w, r)
	}
}

// checkJWT verifies the app token's RS256 signature, issuer and expiry, as GitHub does.
func (f *Fake) checkJWT(tok string) error {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return fmt.Errorf("not a JWT")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(f.key, crypto.SHA256, sum[:], sig); err != nil {
		return fmt.Errorf("bad signature")
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c struct {
		Iss string `json:"iss"`
		Exp int64  `json:"exp"`
		Iat int64  `json:"iat"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	now := time.Now().Unix()
	if c.Iss != f.AppID || c.Exp <= now || c.Iat > now || c.Exp-c.Iat > 600 {
		return fmt.Errorf("bad claims")
	}
	return nil
}

func (f *Fake) getFile(w http.ResponseWriter, r *http.Request) {
	files, ok := f.Branches[r.URL.Query().Get("ref")]
	content, found := files[r.PathValue("path")]
	if !ok || !found {
		reply(w, 404, map[string]any{"message": "Not Found"})
		return
	}
	enc := base64.StdEncoding.EncodeToString([]byte(content))
	reply(w, 200, map[string]any{"type": "file", "encoding": "base64", "content": enc[:len(enc)/2] + "\n" + enc[len(enc)/2:], "sha": BlobSHA(content)})
}

func (f *Fake) putFile(w http.ResponseWriter, r *http.Request) {
	var in struct{ Message, Content, Branch, SHA string }
	json.NewDecoder(r.Body).Decode(&in)
	files, ok := f.Branches[in.Branch]
	if !ok {
		reply(w, 404, map[string]any{"message": "Branch not found"})
		return
	}
	path := r.PathValue("path")
	if old, exists := files[path]; exists != (in.SHA != "") || (exists && BlobSHA(old) != in.SHA) {
		reply(w, 409, map[string]any{"message": "sha does not match"})
		return
	}
	data, err := base64.StdEncoding.DecodeString(in.Content)
	if err != nil {
		reply(w, 422, map[string]any{"message": "bad content"})
		return
	}
	files[path] = string(data)
	f.Writes = append(f.Writes, "put "+in.Branch+" "+path)
	reply(w, 200, map[string]any{})
}

func (f *Fake) getRef(w http.ResponseWriter, r *http.Request) {
	b := r.PathValue("branch")
	if _, ok := f.Branches[b]; !ok {
		reply(w, 404, map[string]any{"message": "Not Found"})
		return
	}
	reply(w, 200, map[string]any{"object": map[string]any{"sha": "commit-" + b}})
}

func (f *Fake) createRef(w http.ResponseWriter, r *http.Request) {
	var in struct{ Ref, SHA string }
	json.NewDecoder(r.Body).Decode(&in)
	name := strings.TrimPrefix(in.Ref, "refs/heads/")
	if _, ok := f.Branches[name]; ok {
		reply(w, 422, map[string]any{"message": "Reference already exists"})
		return
	}
	from, ok := f.Branches[strings.TrimPrefix(in.SHA, "commit-")]
	if !ok {
		reply(w, 422, map[string]any{"message": "Object does not exist"})
		return
	}
	copied := map[string]string{}
	for k, v := range from {
		copied[k] = v
	}
	f.Branches[name] = copied
	f.Writes = append(f.Writes, "branch "+name)
	reply(w, 201, map[string]any{})
}

func (f *Fake) createPR(w http.ResponseWriter, r *http.Request) {
	var in struct{ Title, Body, Head, Base string }
	json.NewDecoder(r.Body).Decode(&in)
	n := len(f.PRs) + 1
	pr := map[string]any{
		"number":   n,
		"title":    in.Title,
		"body":     in.Body,
		"html_url": fmt.Sprintf("https://github.com/%s/%s/pull/%d", f.Owner, f.Name, n),
		"head":     map[string]any{"ref": in.Head},
		"base":     map[string]any{"ref": in.Base},
	}
	f.PRs = append(f.PRs, pr)
	f.Writes = append(f.Writes, "pr "+in.Head)
	reply(w, 201, pr)
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
