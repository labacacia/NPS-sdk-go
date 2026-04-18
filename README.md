English | [中文版](./README.cn.md)

# NPS Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/labacacia/NPS-sdk-go.svg)](https://pkg.go.dev/github.com/labacacia/NPS-sdk-go)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.23%2B-00ADD8)](https://go.dev/)

Go client library for the **Neural Protocol Suite (NPS)** — a complete internet protocol suite purpose-built for AI Agents and models.

Module path: `github.com/labacacia/NPS-sdk-go`

---

## NPS Repositories

| Repo | Role | Language |
|------|------|----------|
| [NPS-Release](https://github.com/labacacia/NPS-Release) | Protocol specifications (authoritative) | Markdown / YAML |
| [NPS-sdk-dotnet](https://github.com/labacacia/NPS-sdk-dotnet) | Reference implementation | C# / .NET 10 |
| [NPS-sdk-py](https://github.com/labacacia/NPS-sdk-py) | Async Python SDK | Python 3.11+ |
| [NPS-sdk-ts](https://github.com/labacacia/NPS-sdk-ts) | Node/browser SDK | TypeScript |
| [NPS-sdk-java](https://github.com/labacacia/NPS-sdk-java) | JVM SDK | Java 21+ |
| [NPS-sdk-rust](https://github.com/labacacia/NPS-sdk-rust) | Async SDK | Rust stable |
| **[NPS-sdk-go](https://github.com/labacacia/NPS-sdk-go)** (this repo) | Go SDK | Go 1.23+ |

---

## Status

**v1.0.0-alpha.1 — Phase 2 initial release**

Covers all five NPS protocols: NCP + NWP + NIP + NDP + NOP. 75 tests passing.

## Requirements

- Go 1.23+ (recommended 1.25)
- Dependencies (managed via `go.mod`):
  - `github.com/vmihailenco/msgpack/v5`
  - `golang.org/x/crypto` (Ed25519, AES-256-GCM)

## Installation

```bash
go get github.com/labacacia/NPS-sdk-go@v1.0.0-alpha.1
```

## Packages

| Import path | Description | Reference |
|-------------|-------------|-----------|
| `.../impl/go/core` | Frame header, codec (Tier-1 JSON / Tier-2 MsgPack), registry, anchor cache, errors | [`doc/nps-go.core.md`](./doc/nps-go.core.md) |
| `.../impl/go/ncp`  | NCP frames: `AnchorFrame`, `DiffFrame`, `StreamFrame`, `CapsFrame`, `ErrorFrame` | [`doc/nps-go.ncp.md`](./doc/nps-go.ncp.md) |
| `.../impl/go/nwp`  | NWP frames: `QueryFrame`, `ActionFrame`; HTTP `NwpClient` | [`doc/nps-go.nwp.md`](./doc/nps-go.nwp.md) |
| `.../impl/go/nip`  | NIP frames: `IdentFrame`, `TrustFrame`, `RevokeFrame`; Ed25519 `NipIdentity` | [`doc/nps-go.nip.md`](./doc/nps-go.nip.md) |
| `.../impl/go/ndp`  | NDP frames: `AnnounceFrame`, `ResolveFrame`, `GraphFrame`; `InMemoryNdpRegistry` + `NdpAnnounceValidator` | [`doc/nps-go.ndp.md`](./doc/nps-go.ndp.md) |
| `.../impl/go/nop`  | NOP frames: `TaskFrame`, `DelegateFrame`, `SyncFrame`, `AlignStreamFrame`; `BackoffStrategy` + `NopTaskStatus` + `NopClient` | [`doc/nps-go.nop.md`](./doc/nps-go.nop.md) |

Full API reference lives under [`doc/`](./doc/) — start with [`doc/overview.md`](./doc/overview.md). For a narrative walkthrough see [`doc/sdk-usage.md`](./doc/sdk-usage.md) / [`doc/sdk-usage.cn.md`](./doc/sdk-usage.cn.md).

## Quick Start

### Encode and decode frames

```go
import (
    "github.com/labacacia/NPS-sdk-go/core"
    "github.com/labacacia/NPS-sdk-go/ncp"
)

registry := core.NewDefaultRegistry()
codec    := core.NewFrameCodec(registry)

schema := ncp.FrameSchema{Fields: []ncp.SchemaField{
    {Name: "id",    Type: "uint64"},
    {Name: "price", Type: "decimal", Semantic: "commerce.price.usd"},
}}
frame := &ncp.AnchorFrame{
    AnchorID: core.ComputeAnchorID(schema),
    Schema:   schema,
    TTL:      3600,
}

wire, _  := codec.Encode(frame)                // MsgPack (Tier-2) by default
decoded, _ := codec.Decode(wire)
```

### Query a Memory Node (NWP)

```go
import "github.com/labacacia/NPS-sdk-go/nwp"

client := nwp.NewClient("http://node.example.com:17433")
caps, err := client.Query(ctx, &nwp.QueryFrame{
    AnchorRef: "sha256:<id>",
    Limit:     50,
})
```

### Ed25519 identity (NIP)

```go
import "github.com/labacacia/NPS-sdk-go/nip"

id, _ := nip.GenerateIdentity()

// Persist with AES-256-GCM + PBKDF2 passphrase
_ = id.Save("node.key", "my-passphrase")

// Load and sign
loaded, _ := nip.LoadIdentity("node.key", "my-passphrase")
sig, _    := loaded.Sign(map[string]any{"nid": "urn:nps:node:example.com:data"})
ok, _     := loaded.Verify(map[string]any{"nid": "urn:nps:node:example.com:data"}, sig)
```

### Announce and resolve (NDP)

```go
import "github.com/labacacia/NPS-sdk-go/ndp"

registry  := ndp.NewInMemoryRegistry()
validator := ndp.NewAnnounceValidator()
validator.RegisterPublicKey(nid, id.PubKeyString())

_ = registry.Announce(frame)
resolved, _ := registry.Resolve("nwp://example.com/data")
```

### Submit a NOP task

```go
import "github.com/labacacia/NPS-sdk-go/nop"

client  := nop.NewClient("http://orchestrator.example.com:17433")
taskID, _ := client.Submit(ctx, &nop.TaskFrame{
    TaskID: "job-1",
    DAG: nop.TaskDAG{
        Nodes: []nop.TaskNode{{ID: "a", Action: "data.fetch", Agent: "urn:nps:node:data.example.com"}},
    },
})
status, _ := client.Wait(ctx, taskID, 30*time.Second)
```

## Encoding Tiers

| Tier | Constant | Description |
|------|----------|-------------|
| Tier-1 | `core.EncodingTierJSON` | UTF-8 JSON — development / interop |
| Tier-2 | `core.EncodingTierMsgPack` | MessagePack — default, ~60% smaller |

## NIP CA Server

A standalone NIP Certificate Authority server is bundled under [`ca-server/`](./ca-server/) — `net/http` stdlib, SQLite-backed, Docker-ready.

## Testing

```bash
go test ./...
```

## License

Apache 2.0 — see [LICENSE](./LICENSE) and [NOTICE](./NOTICE).

Copyright 2026 INNO LOTUS PTY LTD
