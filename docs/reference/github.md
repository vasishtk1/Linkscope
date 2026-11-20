# internal/github

Talks to GitHub as a **GitHub App** so the agent can open a pull request with the redirects. It's phase 5 of the pull request agent. Everything goes through GitHub's REST API, so the repo is never downloaded.

## Why a GitHub App

A GitHub App is installed on chosen repos and gets only the permissions it asks for. Linkscope's app needs:

| Permission | Why |
|---|---|
| Contents: read and write | Read the redirect file, save the changed one on a new branch |
| Pull requests: read and write | List open pull requests (to avoid duplicates) and open one |
| Metadata: read | Required by GitHub for every app |

Unlike a personal access token, it can't touch any other repo, and its commits and pull requests show up as the app instead of as you.

## Signing in

1. **The app's JWT.** `App.JWT` builds a JSON Web Token: a header and claims (`iss` = app ID, issued 1 minute ago to allow for clock differences, expiring in 9 minutes; GitHub allows at most 10), signed with the app's private key using RS256. RS256 means RSA signature over a SHA-256 hash, which Go's standard library does (`crypto/rsa`), so no JWT library is needed.
2. **Find the installation.** `GET /repos/{owner}/{repo}/installation` with the JWT returns the install's ID, so you don't have to copy it from GitHub's settings.
3. **Installation token.** `POST /app/installations/{id}/access_tokens` with the JWT returns a token that works for 1 hour. The request limits it to just that one repo.

Tokens and keys are never printed or logged, and `APIError` only holds GitHub's status and message.

## app.go

### `App`

`ID`, `Key` (the private key), and optional `BaseURL`, `HTTP` and `Now`, which the tests use to point at a fake GitHub and to make an expired token.

### `LoadKey(path string) (*rsa.PrivateKey, error)`

Reads the `.pem` file GitHub gives you. GitHub's keys are PKCS#1 (`RSA PRIVATE KEY`), but PKCS#8 (`PRIVATE KEY`) also works in case the key was converted.

### `(a *App) JWT() (string, error)`

Step 1 above. Each part is base64url-encoded without padding, as the JWT standard requires.

### `(a *App) Repo(ctx, owner, name string) (*Repo, error)`

Steps 2 and 3. A `404` on the installation lookup becomes "the GitHub App isn't installed on owner/name", which is the usual mistake.

## repo.go

### `Repo`, `PR`, `APIError`, `ErrNotFound`, `ErrExists`

`Repo` is one repository plus its token. `PR` has the number, link, title and branch. `APIError` is a non-2xx answer: status plus GitHub's message, with any field errors appended.

### `(r *Repo) DefaultBranch(ctx) (string, error)`

The branch pull requests go into, usually `main`.

### `(r *Repo) File(ctx, path, ref) ([]byte, string, error)`

`GET /contents/{path}?ref=` returns the file base64-encoded (with line breaks, which are removed before decoding) and its blob SHA. The SHA is needed to update the file later: GitHub refuses the update if the file changed in between, so a change made by someone else can't be overwritten by mistake. A missing file, or a folder, is `ErrNotFound`.

### `(r *Repo) BranchSHA(ctx, branch)`, `(r *Repo) CreateBranch(ctx, name, sha)`

The commit a branch points to, and a new branch at that commit (`POST /git/refs`). A taken name is `ErrExists`.

### `(r *Repo) PutFile(ctx, path, branch, message, content, sha)`

`PUT /contents/{path}` saves the file on the branch as one commit. `sha` is the old blob's SHA, or `""` for a new file.

### `(r *Repo) OpenPRs(ctx)`, `(r *Repo) CreatePR(ctx, title, body, head, base)`

Lists open pull requests (first 100) and opens one from `head` into `base`. Nothing here can merge.

### `client.do(ctx, method, path, in, out) error`

Sends one request with GitHub's recommended headers (`Accept: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11-28`, a `User-Agent`, and the token), encodes `in` as JSON, and decodes the answer into `out`. Answers are capped at 4 MB.

### `escapePath(p)`

Escapes each part of a file path or branch name for the URL, keeping the slashes.

## githubtest/fake.go

A tiny GitHub that runs inside the tests (`httptest`), so tests never touch the real one:

- one app installed on one repo, with branches of files kept in memory
- it **checks the JWT like GitHub does**: RS256 signature against the app's public key, issuer, expiry, and at most 10 minutes long
- repo calls need the installation token
- updating a file needs the right blob SHA (`BlobSHA`)
- `Writes` records every change (branch, file save, pull request), so tests can check that a preview wrote nothing

## Tests

`github_test.go`: loading PKCS#1 and PKCS#8 keys and a bad file; the whole flow (install lookup, token, default branch, read, branch, save, pull request, list) leaving `main` unchanged; a taken branch name; a stale SHA; and errors: app not installed, wrong key, expired token, and an error message that never contains the token.
