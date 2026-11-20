package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	API        = "https://api.github.com"
	apiVersion = "2022-11-28"
	userAgent  = "Linkscope"
)

// App is a GitHub App: its ID and private key prove who it is.
type App struct {
	ID      string
	Key     *rsa.PrivateKey
	BaseURL string
	HTTP    *http.Client
	Now     func() time.Time
}

// LoadKey reads the app's private key from the .pem file GitHub gives you.
func LoadKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s: not a PEM file", path)
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, fmt.Errorf("%s: not an RSA key", path)
	}
	return nil, fmt.Errorf("%s: unexpected PEM block %q", path, block.Type)
}

// JWT is the app's short-lived signed token (RS256, 9 minutes), used only to get an installation token.
func (a *App) JWT() (string, error) {
	if a.ID == "" || a.Key == nil {
		return "", errors.New("GitHub App ID and private key are required")
	}
	now := time.Now()
	if a.Now != nil {
		now = a.Now()
	}
	header := `{"alg":"RS256","typ":"JWT"}`
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": a.ID,
	})
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	signed := enc.EncodeToString([]byte(header)) + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.Key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signed + "." + enc.EncodeToString(sig), nil
}

// Repo finds the app's installation on owner/name and returns a client for that repo with a 1-hour token.
func (a *App) Repo(ctx context.Context, owner, name string) (*Repo, error) {
	jwt, err := a.JWT()
	if err != nil {
		return nil, err
	}
	c := client{base: a.base(), http: a.client(), token: jwt}
	var inst struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, "GET", "/repos/"+owner+"/"+name+"/installation", nil, &inst); err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
			return nil, fmt.Errorf("the GitHub App isn't installed on %s/%s (install it on that repo first)", owner, name)
		}
		return nil, err
	}
	var tok struct {
		Token string `json:"token"`
	}
	path := fmt.Sprintf("/app/installations/%d/access_tokens", inst.ID)
	body := map[string]any{"repositories": []string{name}}
	if err := c.do(ctx, "POST", path, body, &tok); err != nil {
		return nil, err
	}
	if tok.Token == "" {
		return nil, errors.New("GitHub returned an empty installation token")
	}
	return &Repo{Owner: owner, Name: name, c: client{base: c.base, http: c.http, token: tok.Token}}, nil
}

func (a *App) base() string {
	if a.BaseURL != "" {
		return strings.TrimRight(a.BaseURL, "/")
	}
	return API
}

func (a *App) client() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}
