# athena-proxy

Plain TCP on localhost for Athena CTF challenge instances. It speaks TLS and SNI to the gateway so your tooling does not have to.

Every challenge instance is reachable at its own hostname on port 443, and the gateway routes on the TLS server name. `athena-proxy` gives each hostname a local address — `127.0.0.1:13370` — and any tool that can open a TCP socket reaches the challenge through it:

```bash
athena-proxy run &
athena-proxy add s8js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz --name pwn-heap
nc 127.0.0.1 13370
```

If your tool speaks TLS itself, it can reach a challenge directly:

```bash
openssl s_client -connect <hostname>:443 -servername <hostname>
ncat --ssl <hostname> 443
```

```python
io = remote("<hostname>", 443, ssl=True)
```

## Install

### One-liner

Linux and macOS:

```bash
curl -fsSL https://downloads.athena-ctf.com/proxy/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://downloads.athena-ctf.com/proxy/install.ps1 | iex
```

Re-running either script upgrades the binary in place. Defaults:

| Platform      | Install location                                        |
| ------------- | ------------------------------------------------------- |
| Linux / macOS | `~/.local/bin/athena-proxy`                             |
| Windows       | `%LOCALAPPDATA%\Programs\athena-proxy\athena-proxy.exe` |

Override with `ATHENA_PROXY_VERSION` (e.g. `v1.0.0`) and `ATHENA_PROXY_INSTALL_DIR`.

### Manual

Download an archive for your platform and verify it against `SHA256SUMS`:

| Platform              | File                                                                             |
| --------------------- | -------------------------------------------------------------------------------- |
| Linux (x86-64)        | `https://downloads.athena-ctf.com/proxy/latest/athena-proxy-linux-x64.tar.gz`    |
| Linux (ARM64)         | `https://downloads.athena-ctf.com/proxy/latest/athena-proxy-linux-arm64.tar.gz`  |
| macOS (Apple silicon) | `https://downloads.athena-ctf.com/proxy/latest/athena-proxy-darwin-arm64.tar.gz` |
| macOS (Intel)         | `https://downloads.athena-ctf.com/proxy/latest/athena-proxy-darwin-x64.tar.gz`   |
| Windows (x86-64)      | `https://downloads.athena-ctf.com/proxy/latest/athena-proxy-windows-x64.zip`     |
| Windows (ARM64)       | `https://downloads.athena-ctf.com/proxy/latest/athena-proxy-windows-arm64.zip`   |

```bash
curl -fsSLO https://downloads.athena-ctf.com/proxy/latest/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
```

Put a version in the path in place of `latest` to pin one — `.../proxy/v1.0.0/athena-proxy-linux-x64.tar.gz`. Every published version stays at its own path, and `.../proxy/versions.json` lists them.

Each archive carries a build provenance attestation, verifiable with the GitHub CLI:

```bash
gh attestation verify athena-proxy-linux-x64.tar.gz --repo astraqcd/athena-proxy
```

### From source

```bash
go install github.com/astraqcd/athena-proxy@latest
```

## Usage

Start the daemon once per session. It serves every registered target at the same time, each on its own local port:

```bash
athena-proxy run
```

Then register the hostname from the event app's connect panel:

```bash
athena-proxy add s8js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz --name pwn-heap
```

| Command                                      | What it does                                       |
| -------------------------------------------- | -------------------------------------------------- |
| `athena-proxy run`                           | Start the daemon and serve every registered target |
| `athena-proxy add <hostname>`                | Register a hostname and print its local address    |
| `athena-proxy list`                          | Show every target and its local address            |
| `athena-proxy remove <name\|hostname\|port>` | Drop a target and close its listener               |

`add` takes `--name <label>` to give the target a readable name, and `--port <local>` to pin its local port.

`run` needs no restart when you register a target — `add`, `list` and `remove` talk to the running daemon over a loopback control port.

### From a script

The local address is a plain TCP endpoint, so nothing needs to know about the tunnel:

```python
from pwn import *

io = remote("127.0.0.1", 13370)
io.sendline(b"payload")
print(io.recvall())
```

## How it works

Each registered hostname gets its own loopback listener. A connection to that listener opens exactly one TLS connection to `<hostname>:443` with the server name set to that hostname, and bytes are pumped both ways until both directions close. There is no pooling, no multiplexing and no connection reuse.

The hostname is the whole configuration: it resolves publicly, it is the address to dial, and it identifies the instance at the gateway.

`add` takes a challenge hostname — a 24-character id, optionally suffixed with `-<port>`, under `tcp.challs.ctf-platform.xyz` — and refuses anything else before opening a listener, so a mistyped or truncated paste fails at registration. Web challenges live under `web.challs.ctf-platform.xyz`. Registering one is refused with the `https://` URL to open in a browser instead, since proxying it would gain you nothing.

Three properties are worth knowing:

- **Half-close propagates.** A script that calls `shutdown(SHUT_WR)` and keeps reading works through the tunnel exactly as it does against a direct connection.
- **There is no idle timeout.** An interactive pwn connection can sit quiet for as long as it likes.
- **A failed connection never takes down a listener.** If an instance has expired, that one connection fails and the local address keeps working; reprovision and the next attempt succeeds with nothing to reconfigure.

Listeners and the control port bind `127.0.0.1`, so targets are reachable from your machine alone.

## State

Registered hostnames, their names, their local ports and the control port live in one file:

| Platform | Path                                                    |
| -------- | ------------------------------------------------------- |
| Linux    | `~/.config/athena-proxy/state.json`                     |
| macOS    | `~/Library/Application Support/athena-proxy/state.json` |
| Windows  | `%AppData%\athena-proxy\state.json`                     |

Set `ATHENA_PROXY_HOME` to put it somewhere else. Local ports are persisted, so a target keeps its local address across a daemon restart. If a persisted port is taken when the daemon starts, it takes another and says so on stderr.

## Building

```bash
go build ./...
go test ./...
./scripts/build-release.sh dev
```

## Licence

Apache-2.0. See [LICENSE](LICENSE).
