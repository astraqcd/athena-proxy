# athena-proxy

A local proxy that gives a plain TCP address on loopback for a remote hostname reachable over TLS, selecting the destination by SNI. One binary, two roles: `athena-proxy run` is a daemon holding one loopback listener per registered hostname; every other invocation is a client of that daemon's loopback control port.

It runs on the hostname the user registers and the domain compiled into `proxy.Domain`. It makes no network call that is not a proxied connection, holds no credential, and reads no configuration beyond its own state file. Keep it that way.

## What breaks silently

- **Half-close must propagate in both directions.** A large class of exploit and protocol scripts calls `shutdown(SHUT_WR)` and keeps reading. `copyHalf` calls `CloseWrite()` on the far side when its source reaches EOF, and the connection finishes only when both directions have closed. Neither direction may cancel the other — no shared context, no `errgroup` that tears down on first return. Wiring the two copies together compiles, passes a naive echo test, and then breaks every such script through the tunnel while it keeps working against a direct connection.
- **There is no idle timeout, deliberately.** The dial and the handshake are bounded; once the handshake completes, no read or write deadline is set, and `tunnel.go` clears the handshake deadline explicitly to guarantee it. Interactive sessions sit quiet for long stretches, so an idle timeout kills live connections with no error a user can act on.
- **The dial and the handshake are two separate steps.** They fail for different reasons — unreachable versus rejected — and `tunnel.go` reports them separately. A `tls.Dialer` merges both into one error.
- **`RootCAs` stays nil in production.** Verification uses the system trust store. The field exists on `proxy.Config` so tests can point at a self-signed echo server.
- **Local ports are persisted, not ephemeral.** A target keeps its local address across a daemon restart. That is the property users build on: register once, keep the same address, never re-point tooling. `portBase` sits below every platform's ephemeral range, so a persisted port is unlikely to have been taken by something else between runs.

## Invariants

- **Loopback only.** Both the per-target listeners and the control port bind `127.0.0.1`. A wildcard bind on an untrusted network turns every registered target into an open relay to the user's own instances, reachable by anyone on that network with no discovery step.
- **A failed connection never takes down a listener.** A registered hostname outlives whatever is behind it. `accept` returns only on `net.ErrClosed`, and a dial or handshake failure ends that one connection — so a target that starts working again needs no re-registration.
- **Targets are independent.** One goroutine per connection, and no lock is held on the data path: `accept` captures its own target, so the registry mutex is touched only by add, list and remove. A hung connection blocks nothing, on its own listener or any other.
- **One inbound connection maps to exactly one outbound TLS connection.** No pooling, no multiplexing, no reuse.
- **Diagnostics go to stderr in plain language.** This runs on a laptop, so there is no structured logging and no log file. `Daemon.errorf` serialises writes so concurrent failures cannot interleave mid-line.

## The control port

`net/http` over loopback TCP, which is portable across all three platforms where a Unix domain socket and a Windows named pipe are not. Its number is recorded in the state file so a second invocation finds the running daemon; when nothing answers, the CLI says so rather than starting a daemon implicitly. `/v1/status` identifies the service, so a stale port now held by something else reads as "no daemon" instead of producing a confusing error.

Any local process can reach a loopback port, and a web page can attempt one. Two guards in `server.go` bound that:

- **`Content-Type: application/json` on every mutation**, so a form-based cross-site request cannot drive the API — an HTML form can only send the three simple content types.
- **A loopback `Host` header**, so a page that resolves its own domain to `127.0.0.1` cannot reach the API by DNS rebinding.

`GET` responses are protected by the same-origin policy, which is why they carry no CORS headers and must not gain any.

## Layout

- `main.go` — cobra command tree; `commands.go` — command bodies and the daemon-connection helper
- `internal/proxy/` — the data path: dial, handshake, half-close pump, hostname validation
- `internal/daemon/` — target registry, listeners, port allocation, control HTTP server
- `internal/control/` — the control API types and the client every non-`run` invocation uses
- `internal/state/` — the state file: its location per platform, and its atomic write
- `internal/tlstest/` — a TLS echo server standing in for the remote; test-only, imported by no production path
- `scripts/build-release.sh` — the `GOOS`/`GOARCH` matrix, archives and `SHA256SUMS`; `scripts/third-party-notices.sh` — the `THIRD-PARTY-NOTICES` staged into each one

## Conventions

```
go build ./...
go test -race ./...
golangci-lint run ./...
./scripts/build-release.sh dev
```

Gates are `gofmt`, `go vet` and `golangci-lint run`, all three run by CI on every push alongside tests on Linux, macOS and Windows. Add dependencies with `go get`, never by hand-editing `go.mod`.

**Code carries no informational or explanatory comments** — only genuine `TODO` / `FIXME` markers. That covers godoc-style `// FuncName does …` comments, godoc convention notwithstanding. Rationale goes in the commit message, or in this file. Greenfield files are the risky case: with no surrounding code to calibrate against, the urge to narrate is strongest on exactly the lines that must not carry a comment.

Before inventing a helper, type alias or signature shape, copy the one already in the tree. `proxy.Config` with its `withDefaults` is the worked example for optional configuration, and the zero value is always the production default.

## Releases

**A tag is the whole release.** A `v*` tag drives `.github/workflows/release.yml`: tests on all three platforms, then one Ubuntu runner cross-compiles every target, attests build provenance against `SHA256SUMS`, and uploads `dist/` to `athena-downloads` under `proxy/<tag>/` before rewriting `proxy/latest.json` and prepending the release to `proxy/versions.json`. The tag and that upload are the entire release: `downloads.athena-ctf.com/proxy/…` is the only place a user gets a binary. `scripts/build-release.sh` is the same script CI and a developer run, so a local build reproduces the release layout exactly.

The version is injected with `-X main.version=<tag>`; a build without it reports `dev`. It reaches no filename: an archive is `athena-proxy-<os>-<arch>.<ext>`, and the version lives in the key path alone.

⚠ **That grammar is fixed, and this repository cannot see what depends on it.** An archive named anything else, or a new `GOOS`/`GOARCH` target added here alone, uploads cleanly and then 404s for every user — the download surface serves an allowlist it maintains separately. Changing a name or adding a target is a coordinated change; raise it before cutting the tag.

Each archive carries `LICENSE` and a `THIRD-PARTY-NOTICES` written by `scripts/third-party-notices.sh`, which pins its own `go-licenses` version and installs it to a temp `GOBIN`, so `go.mod` stays at the three modules the binary actually links. It reports under `GOOS=windows`, the superset of the three build graphs — `mousetrap` reaches the binary on Windows alone and would otherwise be missing from the notice shipped inside the Windows archive.

`versions.json` is the only record of release history, because nothing can list the bucket. The upload step reads it, prepends this release and writes it back; that read-modify-write is safe because this job is the only writer.

The upload reads `R2_DOWNLOADS_ACCESS_KEY_ID`, `R2_DOWNLOADS_SECRET_ACCESS_KEY` and `R2_ENDPOINT` from the `release` environment, which admits `v*` tags alone. This repository is public, so those credentials stay out of reach of anything a fork can trigger: no `pull_request_target`, `workflow_run` or `issue_comment` trigger, ever. Every action in that job is first-party; adding a third-party one means pinning it to a commit SHA.
