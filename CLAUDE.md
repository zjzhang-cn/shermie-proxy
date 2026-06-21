# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o proxy .    # Build the proxy binary
go run Main.go --port 9090 --nagle=true   # Run directly
```

No Makefile or test suite exists. There are no tests in this project.

## Architecture

A **single-port multi-protocol proxy** that auto-detects the inbound protocol by peeking at the first byte of each connection, then delegates to the appropriate handler.

### Protocol Detection Flow

`Core/ProxyServer.go:174-201` — `handle()` peeks at the first 3 bytes of a connection:

| First byte | Interpreted as | Handler |
|---|---|---|
| `G`, `P`, `D`, `O`, `H`, `C` | HTTP method | `ProxyHttp` |
| `0x05` | SOCKS5 version | `ProxySocks5` |
| Anything else | Raw TCP | `ProxyTcp` |

### Key Modules

- **`Core/ProxyHttp.go`** — Handles HTTP, HTTPS, WS, and WSS. For CONNECT (HTTPS/WSS), it does a MITM: generates a per-host TLS certificate signed by the root CA (`Core/Cache.go` handles deduplication for concurrent cert generation), terminates the client TLS, then forwards the decrypted request. For WebSocket, it upgrades both sides and bidirectionally relays frames.
- **`Core/ProxySocks5.go`** — SOCKS5 handshake (no auth), then bidirectional relay for TCP or UDP `CONNECT`/`BIND`/`UDP ASSOCIATE` commands.
- **`Core/ProxyTcp.go`** — Raw TCP proxy with optional TLS wrapping; used when the `--to` flag specifies a fixed destination, or for non-HTTP/non-SOCKS5 traffic.
- **`Core/Certificate.go`** — Root CA generation (creates `cert.crt`/`cert.key` if missing), per-host certificate generation signed by the root CA. The global `Core.Cert` variable is initialized on startup.
- **`Contract/IServerProcesser.go`** — Single-method interface `Handle()` implemented by all protocol handlers.
- **`Utils/`** — Platform-specific files: `Windows.go` (system proxy config via WinInet API, cert store installation), `Linux.go` (stubs returning `errors.New("不支持Linux系统")`). `Utils.go` has cross-platform helpers.

### Event Hook System

`ProxyServer` exposes callback fields (`OnHttpRequestEvent`, `OnHttpResponseEvent`, `OnWsRequestEvent`, etc.) that users set to intercept and modify traffic. Each event receives a `resolve` function — calling it forwards the (potentially modified) data. Returning `false` from HTTP events stops processing (for manual conn handling).

### Connection Model

`Core/ConnPeer.go` — `ConnPeer` struct embeds `net.Conn`, `*bufio.Reader`, `*bufio.Writer`, and `*ProxyServer`. Each protocol handler embeds `ConnPeer` and adds protocol-specific state.

### Key Dependencies

- `github.com/viki-org/dnscache` — DNS caching resolver (5-minute TTL)
- `golang.org/x/sys` — Windows system calls for proxy configuration
- The `Core/Websocket/` package is a forked/embedded gorilla/websocket

### Startup Flow

1. `Log.NewLogger().Init()` — init stdout logger
2. `Core.NewCertificate().Init()` — load or generate root CA
3. Parse `--port`, `--nagle`, `--proxy`, `--to`, `--network` flags
4. Multiple ports supported: comma-separated `--port` and `--network` spawn separate listeners via goroutines
5. Each listener runs 5 accept goroutines in `MultiListen()`

## Notable Patterns

- **The root CA cert/key files are read/written relative to CWD** (`./cert.crt`, `./cert.key`), not relative to the binary.
- **Windows-specific features** (system proxy, cert installation) are gated by build tags; Linux stubs return "unsupported" errors.
- **Certificate cache** (`Cache.GetCertificate`) serializes generation per hostname to avoid duplicate cert creation under concurrent requests to the same host.
