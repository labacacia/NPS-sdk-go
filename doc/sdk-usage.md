English | [中文版](./sdk-usage.cn.md)

# NPS Go SDK — Usage Guide

> Copyright 2026 INNO LOTUS PTY LTD — Licensed under Apache 2.0

**Version**: v1.0.0-alpha.1 | **Module**: `github.com/labacacia/NPS-sdk-go` | **Go**: 1.25+

---

## Table of Contents

- [Installation](#installation)
- [Packages](#packages)
- [Quick Start](#quick-start)
- [API Reference](#api-reference)
- [Testing](#testing)

---

## Installation

```bash
go get github.com/labacacia/NPS-sdk-go
```

**Dependencies**

| Dependency | Purpose |
|---|---|
| `github.com/vmihailenco/msgpack/v5` | Tier-2 MsgPack encoding |
| `golang.org/x/crypto` | Ed25519 key derivation (PBKDF2) |

---

## Packages

| Package | Protocol | Description |
|---|---|---|
| `core` | NCP base | Frame types, header codec, frame registry, AnchorFrame cache |
| `ncp` | NCP | AnchorFrame, DiffFrame, StreamFrame, CapsFrame, ErrorFrame |
| `nwp` | NWP | QueryFrame, ActionFrame, NwpClient (HTTP mode) |
| `nip` | NIP | IdentFrame, TrustFrame, RevokeFrame, NipIdentity (Ed25519 + AES-256-GCM) |
| `ndp` | NDP | AnnounceFrame, ResolveFrame, GraphFrame, InMemoryNdpRegistry, NdpAnnounceValidator |
| `nop` | NOP | TaskFrame, DelegateFrame, SyncFrame, AlignStreamFrame, NopClient |

---

## Quick Start

### NCP — Frame Encode / Decode

```go
import (
    "github.com/labacacia/NPS-sdk-go/core"
    "github.com/labacacia/NPS-sdk-go/ncp"
)

// Build an AnchorFrame
frame := &ncp.AnchorFrame{
    AnchorID: "anchor-001",
    Schema:   core.FrameDict{"fields": []string{"id", "name", "score"}},
    TTL:      3600,
}

// Encode with MsgPack (Tier-2, recommended for production)
codec := core.NewNpsFrameCodec(core.CreateFullRegistry())
wire, err := codec.Encode(frame.FrameType(), frame.ToDict(), core.EncodingTierMsgPack, true)
if err != nil {
    log.Fatal(err)
}

// Decode
ft, dict, err := codec.Decode(wire)
if err != nil {
    log.Fatal(err)
}
decoded := ncp.AnchorFrameFromDict(dict)
fmt.Println(ft, decoded.AnchorID) // 0x01 anchor-001
```

### NWP — HTTP Mode Client

```go
import (
    "context"
    "github.com/labacacia/NPS-sdk-go/nwp"
)

client := nwp.NewNwpClient("http://localhost:17433")

// Send AnchorFrame to /anchor
anchor := &ncp.AnchorFrame{
    AnchorID: "schema-v1",
    Schema:   core.FrameDict{"type": "agent-profile"},
    TTL:      3600,
}
if err := client.SendAnchor(ctx, anchor); err != nil {
    log.Fatal(err)
}

// Query: POST /query -> CapsFrame
qf := &nwp.QueryFrame{
    QueryID:  "q-001",
    AnchorID: "schema-v1",
}
caps, err := client.Query(context.Background(), qf)
if err != nil {
    log.Fatal(err)
}
fmt.Println(caps.Caps)

// Invoke action: POST /invoke
af := &nwp.ActionFrame{
    ActionID: "act-001",
    AnchorID: "schema-v1",
    Method:   "compute",
    Params:   core.FrameDict{"input": "hello"},
}
result, err := client.Invoke(context.Background(), af)
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.JSON)
```

### NIP — Identity & Signing

```go
import (
    "github.com/labacacia/NPS-sdk-go/nip"
    "github.com/labacacia/NPS-sdk-go/core"
)

// Generate a new Ed25519 identity
id, err := nip.Generate()
if err != nil {
    log.Fatal(err)
}
fmt.Println(id.PubKeyString()) // "ed25519:<hex>"

// Sign a payload
payload := core.FrameDict{"nid": "agent-001", "action": "register"}
sig := id.Sign(payload)
fmt.Println(sig) // "ed25519:<base64>"

// Verify the signature
ok := id.Verify(payload, sig)
fmt.Println(ok) // true

// Persist key with AES-256-GCM + PBKDF2 encryption
err = id.SaveToFile("/data/agent.key.enc", "my-secret-passphrase")
if err != nil {
    log.Fatal(err)
}

// Load key from file
id2, err := nip.LoadFromFile("/data/agent.key.enc", "my-secret-passphrase")
if err != nil {
    log.Fatal(err)
}

// Build an IdentFrame
ident := &nip.IdentFrame{
    NID:       "agent-001",
    PubKey:    id.PubKeyString(),
    Algorithm: "ed25519",
}
fmt.Println(ident.FrameType()) // 0x20
```

### NDP — Service Discovery

```go
import (
    "github.com/labacacia/NPS-sdk-go/ndp"
)

// In-memory registry with TTL eviction
registry := ndp.NewInMemoryNdpRegistry()

// Announce a node
announce := &ndp.AnnounceFrame{
    NID:      "node-001",
    Host:     "10.0.0.5",
    Port:     17433,
    Protocol: "nwp",
    TTL:      300,
}
registry.Announce(announce)

// Resolve a node by NID
result := registry.Resolve("node-001")
if result != nil {
    fmt.Printf("host=%s port=%d\n", result.Host, result.Port)
}

// Validate an announce frame
validator := ndp.NewNdpAnnounceValidator()
if err := validator.Validate(announce); err != nil {
    log.Fatal(err)
}
```

### NOP — Orchestration Client

```go
import (
    "context"
    "github.com/labacacia/NPS-sdk-go/nop"
)

client := nop.NewNopClient("http://localhost:17433")

// Submit a task
task := &nop.TaskFrame{
    TaskID:   "task-001",
    AnchorID: "schema-v1",
    Method:   "summarise",
    Params:   map[string]any{"text": "Neural Protocol Suite overview"},
}
taskID, err := client.Submit(context.Background(), task)
if err != nil {
    log.Fatal(err)
}

// Poll task status
status, err := client.GetStatus(context.Background(), taskID)
if err != nil {
    log.Fatal(err)
}
fmt.Println(status.Status, status.Result)
```

---

## API Reference

### `core` Package

| Symbol | Description |
|---|---|
| `FrameType` | Frame type constants (0x01–0xFE) |
| `EncodingTierJSON` / `EncodingTierMsgPack` | Encoding tier 1 (JSON) / 2 (MsgPack) |
| `FrameDict` | `map[string]any` — frame payload type |
| `NpsFrameCodec` | Encode / Decode NPS wire format |
| `NewNpsFrameCodec(reg)` | Create codec with a frame registry |
| `CreateFullRegistry()` | Registry with all 17 known frame types |
| `ParseFrameHeader(buf)` | Parse a 6-byte or 10-byte frame header |
| `InMemoryAnchorCache` | TTL-based AnchorFrame cache |

### `ncp` Package

| Symbol | Description |
|---|---|
| `AnchorFrame` | Schema declaration frame (0x01) |
| `DiffFrame` | Schema diff frame (0x02) |
| `StreamFrame` | Streaming payload frame (0x03) |
| `CapsFrame` | Capabilities response frame (0x04) |
| `ErrorFrame` | Unified error frame (0xFE) |
| `*FromDict(d)` | Deserialise each frame type from `FrameDict` |

### `nwp` Package

| Symbol | Description |
|---|---|
| `QueryFrame` | HTTP query frame (0x10) |
| `ActionFrame` | HTTP action invocation frame (0x11) |
| `NwpClient` | HTTP-mode NWP client |
| `NewNwpClient(baseURL)` | Create client with MsgPack + default HTTP |
| `client.SendAnchor(ctx, frame)` | POST anchor to `/anchor` |
| `client.Query(ctx, frame)` | POST to `/query`, returns `CapsFrame` |
| `client.Stream(ctx, frame)` | POST to `/stream`, returns `[]StreamFrame` |
| `client.Invoke(ctx, frame)` | POST to `/invoke`, returns `InvokeResult` |

### `nip` Package

| Symbol | Description |
|---|---|
| `NipIdentity` | Ed25519 signing identity |
| `Generate()` | Create new random identity |
| `LoadFromFile(path, passphrase)` | Load AES-256-GCM encrypted key |
| `id.SaveToFile(path, passphrase)` | Save key with AES-256-GCM + PBKDF2 |
| `id.Sign(payload)` | Sign `FrameDict`, returns `"ed25519:<base64>"` |
| `id.Verify(payload, sig)` | Verify signature |
| `id.PubKeyString()` | Returns `"ed25519:<hex>"` |
| `IdentFrame` | Identity declaration frame (0x20) |
| `TrustFrame` | Trust attestation frame (0x21) |
| `RevokeFrame` | Revocation frame (0x22) |

### `ndp` Package

| Symbol | Description |
|---|---|
| `AnnounceFrame` | Service announcement frame (0x30) |
| `ResolveFrame` | Resolution request frame (0x31) |
| `GraphFrame` | Topology graph frame (0x32) |
| `InMemoryNdpRegistry` | In-memory registry with TTL eviction |
| `registry.Announce(frame)` | Store or remove (ttl=0) an entry |
| `registry.Resolve(nid)` | Resolve NID to `ResolveResult` |
| `NdpAnnounceValidator` | Validate announce frame fields |

### `nop` Package

| Symbol | Description |
|---|---|
| `TaskFrame` | Orchestration task frame (0x40) |
| `DelegateFrame` | Delegation frame (0x41) |
| `SyncFrame` | Sync frame (0x42) |
| `AlignStreamFrame` | Align stream frame (0x43) |
| `NopClient` | HTTP-mode NOP client |
| `NewNopClient(baseURL)` | Create client with default HTTP |
| `client.Submit(ctx, frame)` | POST task to `/tasks`, returns task ID |
| `client.GetStatus(ctx, taskID)` | GET `/tasks/{id}`, returns `NopTaskStatus` |

---

## Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test github.com/labacacia/NPS-sdk-go/ncp -v

# Run with race detector
go test -race ./...
```

The SDK ships with **75 tests** covering all 5 protocol packages.

---

*Default port: **17433** (shared across all NPS protocols)*
