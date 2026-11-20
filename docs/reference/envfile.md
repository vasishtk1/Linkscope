# internal/envfile

Reads one setting, such as `GEMINI_API_KEY` or `GITHUB_APP_ID`, so commands can be configured without putting secrets on the command line, where they'd end up in shell history.

## envfile.go

### `Get(path, name string) string`

Returns the environment variable `name` if it's set. Otherwise it reads the `.env` file at `path` line by line and returns the value after `name=`, without surrounding quotes. It returns `""` if neither has it, so the caller decides what message to show.

The environment wins so a server can pass settings through systemd's `EnvironmentFile` without a `.env` next to the binary.

`.env` is in `.gitignore`, and a local pre-commit hook blocks committing it.

## Tests

`envfile_test.go`: reading from the file (quotes removed, a name that only starts the same way isn't matched), the environment winning, and a missing file.
