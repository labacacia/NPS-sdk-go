English | [中文版](./README.cn.md)

# NPS Go SDK v1.0.0-alpha.3

Go reference implementation of the Neural Protocol Suite (NPS) — covers all five sub-protocols: **NCP · NWP · NIP · NDP · NOP**.

| | |
|---|---|
| **Module** | `github.com/labacacia/NPS-sdk-go` |
| **Go** | 1.25+ |
| **Tests** | 75 passing |
| **License** | Apache 2.0 |

---

## Packages

| Package | Protocol | Description |
|---------|----------|-------------|
| `core` | NCP | Frame types, header codec, registry, AnchorFrame cache |
| `ncp` | NCP | AnchorFrame, DiffFrame, StreamFrame, CapsFrame, HelloFrame, ErrorFrame |
| `nwp` | NWP | QueryFrame, ActionFrame, NwpClient (HTTP mode) |
| `nip` | NIP | IdentFrame, TrustFrame, RevokeFrame, NipIdentity (Ed25519) |
| `ndp` | NDP | AnnounceFrame, ResolveFrame, GraphFrame, InMemoryNdpRegistry, NdpAnnounceValidator |
| `nop` | NOP | TaskFrame, DelegateFrame, SyncFrame, AlignStreamFrame, NopClient |

---

## Installation

```bash
go get github.com/labacacia/NPS-sdk-go
```

---

## Quick Start

### NCP — Frame Codec

```go
import (
    "github.com/labacacia/NPS-sdk-go/core"
    "github.com/labacacia/NPS-sdk-go/ncp"
)

// Create a full registry (all 5 protocols)
reg := core.CreateFullRegistry()
codec := core.NewNpsFrameCodec(reg)

// Build and encode an AnchorFrame
frame := &ncp.AnchorFrame{
    AnchorID: "sha256:abc...",
    Schema:   core.FrameDict{"type": "object", "version": "1"},
    TTL:      3600,
}
wire, err := codec.Encode(frame.FrameType(), frame.ToDict(), core.EncodingTierMsgPack, true)
// wire is ready to send over the network

// Decode on the receiving end
ft, dict, err := codec.Decode(wire)
received := ncp.AnchorFrameFromDict(dict)
```

### NCP — AnchorFrame Cache

```go
cache := core.NewAnchorFrameCache()

// Store with TTL
schema := core.FrameDict{"type": "object", "fields": []any{"name", "value"}}
anchorID, err := cache.Set(schema, 3600) // 1-hour TTL

// Retrieve (returns nil if expired)
schema, err = cache.GetRequired(anchorID)
```

### NWP — HTTP Client

```go
import "github.com/labacacia/NPS-sdk-go/nwp"

client := nwp.NewNwpClient("http://node.example.com:17433")

// Query
qf := &nwp.QueryFrame{AnchorRef: "sha256:abc...", Filters: map[string]any{"status": "active"}}
capsFrame, err := client.Query(ctx, qf)

// Streaming
frames, err := client.Stream(ctx, qf)
for _, sf := range frames {
    fmt.Println(sf.Payload)
}

// Invoke (sync action)
af := &nwp.ActionFrame{Action: "create", Payload: map[string]any{"name": "item"}}
result, err := client.Invoke(ctx, af)

// Async invoke
af.Async = true
result, err = client.Invoke(ctx, af)
fmt.Println(result.Async.TaskID)
```

### NIP — Identity & Signing

```go
import "github.com/labacacia/NPS-sdk-go/nip"

// Generate a new identity
id, err := nip.Generate()
fmt.Println(id.PubKeyString()) // "ed25519:<hex>"

// Sign a frame dict
payload := core.FrameDict{"nid": "urn:nps:node:example.com:agent", "pub_key": id.PubKeyString()}
sig := id.Sign(payload)

// Verify
ok := id.Verify(payload, sig)

// Verify with just the public key string (no private key needed)
ok = nip.VerifyWithPubKeyStr(payload, "ed25519:<hex>", sig)

// Save / Load (AES-256-GCM + PBKDF2-SHA256, 600k iterations)
err = id.Save("/path/to/identity.json", "my-passphrase")
loaded, err := nip.Load("/path/to/identity.json", "my-passphrase")
```

### NDP — Discovery Registry

```go
import "github.com/labacacia/NPS-sdk-go/ndp"

registry := ndp.NewInMemoryNdpRegistry()

// Announce a node
frame := &ndp.AnnounceFrame{
    NID:       "urn:nps:node:example.com:agent",
    Addresses: []map[string]any{{"host": "example.com", "port": uint64(17433), "protocol": "nps"}},
    Caps:      []string{"nwp", "nop"},
    TTL:       300,
    Timestamp: time.Now().UTC().Format(time.RFC3339),
}
registry.Announce(frame)

// Resolve a target URL
result := registry.Resolve("nwp://example.com/agent")
// result.Host, result.Port, result.Protocol

// Validate announce signature
validator := ndp.NewNdpAnnounceValidator()
validator.RegisterPublicKey("urn:nps:node:example.com:agent", "ed25519:<hex>")
frame.Signature = id.Sign(frame.UnsignedDict())
result := validator.Validate(frame)
// result.IsValid, result.ErrorCode, result.Message
```

### NOP — Orchestration Client

```go
import "github.com/labacacia/NPS-sdk-go/nop"

client := nop.NewNopClient("http://orchestrator.example.com:17433")

// Submit a DAG task
tf := &nop.TaskFrame{
    TaskID:    "task-" + uuid,
    DAG:       map[string]any{...},
    TimeoutMs: &timeout,
}
taskID, err := client.Submit(ctx, tf)

// Poll status
status, err := client.GetStatus(ctx, taskID)
fmt.Println(status.State()) // "running"

// Wait for completion (polls every 500ms)
status, err = client.Wait(ctx, taskID, nil)
fmt.Println(status.State())        // "completed"
fmt.Println(status.NodeResults())  // map[string]any

// Cancel
err = client.Cancel(ctx, taskID)
```

---

## Frame Types

| Frame | Type Code | Package |
|-------|-----------|---------|
| AnchorFrame | 0x01 | `ncp` |
| DiffFrame | 0x02 | `ncp` |
| StreamFrame | 0x03 | `ncp` |
| CapsFrame | 0x04 | `ncp` |
| HelloFrame | 0x06 | `ncp` |
| QueryFrame | 0x10 | `nwp` |
| ActionFrame | 0x11 | `nwp` |
| IdentFrame | 0x20 | `nip` |
| TrustFrame | 0x21 | `nip` |
| RevokeFrame | 0x22 | `nip` |
| AnnounceFrame | 0x30 | `ndp` |
| ResolveFrame | 0x31 | `ndp` |
| GraphFrame | 0x32 | `ndp` |
| TaskFrame | 0x40 | `nop` |
| DelegateFrame | 0x41 | `nop` |
| SyncFrame | 0x42 | `nop` |
| AlignStreamFrame | 0x43 | `nop` |
| ErrorFrame | 0xFE | `ncp` |

---

## Encoding

| Tier | Constant | Notes |
|------|----------|-------|
| JSON | `core.EncodingTierJSON` | Human-readable, Tier-1 |
| MsgPack | `core.EncodingTierMsgPack` | ~60% size reduction, Tier-2 (default) |

---

## Backoff Strategies (NOP)

```go
delay := nop.ComputeDelayMs(nop.BackoffExponential, 100, 5000, attempt)
// BackoffFixed, BackoffLinear, BackoffExponential
```

---

## Running Tests

```bash
go test ./...
```

---

## License

Apache 2.0 — Copyright 2026 INNO LOTUS PTY LTD
