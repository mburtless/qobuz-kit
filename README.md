# qobuz-kit

A command-line toolkit for Qobuz playlists.

qobuz-kit currently lets you bulk favorite playlist tracks through an interactive, searchable playlist picker. Choose a playlist, review the number of new hearts, and confirm before anything is added. Existing favorites and duplicate tracks are skipped, and your playlists stay unchanged.

This is an unofficial tool that uses Qobuz's session API and is not affiliated with Qobuz.

## Features

- Search and select playlists returned by your Qobuz account.
- Preview changes with `--dry-run`.
- Skip existing favorites, duplicate tracks, and tracks marked unavailable.
- Verify each added heart against Qobuz's favorite status.
- Process multiple playlists in one session.
- Rerun after an interruption to add any remaining favorites.

## Getting started

You'll need Go 1.23 or newer, a Qobuz account, and an interactive terminal.

### Build from source

From the repository directory:

```sh
go build -o bin/qobuz-kit ./cmd/qobuz-kit
```

### Set up authentication

qobuz-kit uses credentials from an existing Qobuz web session. It does not provide a login flow or automatically extract browser credentials.

1. Sign in to the [Qobuz web player](https://play.qobuz.com).
2. Open your browser's developer tools, select **Network**, and load a playlist or your favorites.
3. Find a request to `www.qobuz.com/api.json/0.2/`.
4. Copy the app ID and user authentication token from the **same session**. Look for the `X-App-Id` and `X-User-Auth-Token` request headers, or the `app_id` and `user_auth_token` parameters.
5. Set `QOBUZ_APP_ID` and `QOBUZ_USER_AUTH_TOKEN` in your shell.

In Bash or Zsh, export the values you copied:

```sh
export QOBUZ_APP_ID='your-app-id'
export QOBUZ_USER_AUTH_TOKEN='your-session-token'
```

Treat the token like a password: do not commit it or include it in an issue. The CLI keeps credentials in memory and does not print or save them.

### Preview and run

Start with a dry run to review a playlist without changing your account:

```sh
./bin/qobuz-kit --dry-run
```

To add favorites:

```sh
./bin/qobuz-kit
```

Select a playlist and review the counts for duplicates, existing favorites, unavailable tracks, invalid entries, and new hearts to add. Confirm **Yes** to proceed; the confirmation defaults to **No**.

## Usage

The CLI runs in a full-screen terminal interface. Both stdin and stderr must be connected to an interactive terminal, including for dry runs.

| Control | Action |
| --- | --- |
| Type in the picker | Filter playlists |
| ↑ / ↓ | Move through playlists |
| Enter | Select a playlist |
| Esc / Ctrl+U | Clear the search |
| q | Quit from the picker or confirmation prompt |
| Ctrl+C | Interrupt the session |

The `q` key is reserved for quitting, including while searching. You can also select **Quit** in the picker.

After processing a playlist, the CLI returns to the picker. Declining confirmation or selecting a playlist with no new hearts also returns to the picker. A dry run previews one playlist and exits.

### Options

| Flag | Description | Default |
| --- | --- | --- |
| `--dry-run` | Preview missing hearts without changing your account | `false` |
| `--color` | Color mode: `auto`, `always`, or `never` | `always` |
| `--help` | Show command help | |

Use `--color=auto` to detect color support on stderr and respect `NO_COLOR`, or `--color=never` to disable colors.

## Troubleshooting

- **Authentication failed:** get a fresh session token and its matching app ID from the web player.
- **Interactive terminal required:** run directly in a terminal with stdin and stderr attached, rather than piping or redirecting them.
- **Some tracks failed:** individual failures are reported with track IDs, and processing continues. Select the playlist again or rerun the CLI to retry missing hearts. Favorites are refreshed for each selection.
- **Rate limit reached:** the CLI retries rate-limited requests, then stops if retries are exhausted. Try again later. Authentication errors also stop the session.
- **Interrupted session:** rerun to recompute the remaining hearts. Already added favorites are skipped; no local checkpoint is needed.
- **Unavailable or invalid tracks:** tracks explicitly marked non-streamable are skipped without counting as failures. Entries without valid track IDs count as incomplete processing.

Qobuz's API behavior can change. If you're trying the tool for the first time, use a small playlist and preview it with `--dry-run`.

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Session ended successfully |
| `1` | An error occurred or processing was incomplete during the session |
| `130` | Session was interrupted |

## Development

```sh
go test ./...
go vet ./...
```

### Releases

GitHub Actions runs tests and vet, then uses GoReleaser to publish a GitHub release when a version tag such as `v0.1.0` is pushed. Releases include binaries for Linux, macOS, and Windows on amd64 and arm64, packaged with this README, plus SHA-256 checksums. Tags such as `v0.1.0-rc.1` produce prereleases.

After committing and pushing the release changes, create and push the version tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The workflow uses GitHub's built-in `GITHUB_TOKEN`; no additional release secret is needed.

With GoReleaser v2.18.3 installed, validate the configuration and build release archives locally without publishing:

```sh
goreleaser check
goreleaser release --snapshot --clean
```

Local release artifacts are written to `dist/`.

Run from source with the Makefile:

```sh
make run
make run ARGS="--dry-run --color=auto"
```

Automated tests use local HTTP fixtures and do not change a Qobuz account.

The CLI uses [Cobra](https://github.com/spf13/cobra) for commands and flags, and [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Huh](https://github.com/charmbracelet/huh) for the terminal interface. The API client follows the playlist and favorite endpoints implemented by [Minim](https://github.com/bbye98/minim/blob/main/src/minim/qobuz.py).
