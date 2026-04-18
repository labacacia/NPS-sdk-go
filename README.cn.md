[English Version](./README.md) | 中文版

# NPS Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/labacacia/nps/impl/go.svg)](https://pkg.go.dev/github.com/labacacia/nps/impl/go)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.23%2B-00ADD8)](https://go.dev/)

**Neural Protocol Suite (NPS)** 的 Go 客户端库 —— 专为 AI Agent 与神经模型设计的完整互联网协议族。

模块路径：`github.com/labacacia/nps/impl/go`

---

## NPS 仓库导航

| 仓库 | 职责 | 语言 |
|------|------|------|
| [NPS-Release](https://github.com/labacacia/NPS-Release) | 协议规范（权威来源） | Markdown / YAML |
| [NPS-sdk-dotnet](https://github.com/labacacia/NPS-sdk-dotnet) | 参考实现 | C# / .NET 10 |
| [NPS-sdk-py](https://github.com/labacacia/NPS-sdk-py) | 异步 Python SDK | Python 3.11+ |
| [NPS-sdk-ts](https://github.com/labacacia/NPS-sdk-ts) | Node/浏览器 SDK | TypeScript |
| [NPS-sdk-java](https://github.com/labacacia/NPS-sdk-java) | JVM SDK | Java 21+ |
| [NPS-sdk-rust](https://github.com/labacacia/NPS-sdk-rust) | 异步 SDK | Rust stable |
| **[NPS-sdk-go](https://github.com/labacacia/NPS-sdk-go)**（本仓库） | Go SDK | Go 1.23+ |

---

## 状态

**v1.0.0-alpha.1 — Phase 2 首个发布**

覆盖 NPS 全部五个协议：NCP + NWP + NIP + NDP + NOP，75 个测试全部通过。

## 运行要求

- Go 1.23+（推荐 1.25）
- 依赖（通过 `go.mod` 管理）：
  - `github.com/vmihailenco/msgpack/v5`
  - `golang.org/x/crypto`（Ed25519、AES-256-GCM）

## 安装

```bash
go get github.com/labacacia/nps/impl/go@v1.0.0-alpha.1
```

## 包

| 引用路径 | 说明 | 参考文档 |
|----------|------|----------|
| `.../impl/go/core` | 帧头、编解码（Tier-1 JSON / Tier-2 MsgPack）、注册表、AnchorFrame 缓存、错误类型 | [`doc/nps-go.core.cn.md`](./doc/nps-go.core.cn.md) |
| `.../impl/go/ncp`  | NCP 帧：`AnchorFrame`、`DiffFrame`、`StreamFrame`、`CapsFrame`、`ErrorFrame` | [`doc/nps-go.ncp.cn.md`](./doc/nps-go.ncp.cn.md) |
| `.../impl/go/nwp`  | NWP 帧：`QueryFrame`、`ActionFrame`；HTTP `NwpClient` | [`doc/nps-go.nwp.cn.md`](./doc/nps-go.nwp.cn.md) |
| `.../impl/go/nip`  | NIP 帧：`IdentFrame`、`TrustFrame`、`RevokeFrame`；Ed25519 `NipIdentity` | [`doc/nps-go.nip.cn.md`](./doc/nps-go.nip.cn.md) |
| `.../impl/go/ndp`  | NDP 帧：`AnnounceFrame`、`ResolveFrame`、`GraphFrame`；`InMemoryNdpRegistry` + `NdpAnnounceValidator` | [`doc/nps-go.ndp.cn.md`](./doc/nps-go.ndp.cn.md) |
| `.../impl/go/nop`  | NOP 帧：`TaskFrame`、`DelegateFrame`、`SyncFrame`、`AlignStreamFrame`；`BackoffStrategy` + `NopTaskStatus` + `NopClient` | [`doc/nps-go.nop.cn.md`](./doc/nps-go.nop.cn.md) |

完整 API 参考见 [`doc/`](./doc/) —— 从 [`doc/overview.cn.md`](./doc/overview.cn.md) 入门。叙事性教程参见 [`doc/sdk-usage.cn.md`](./doc/sdk-usage.cn.md) / [`doc/sdk-usage.md`](./doc/sdk-usage.md)。

## 快速开始

### 编解码帧

```go
import (
    "github.com/labacacia/nps/impl/go/core"
    "github.com/labacacia/nps/impl/go/ncp"
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

wire, _  := codec.Encode(frame)                // 默认 MsgPack（Tier-2）
decoded, _ := codec.Decode(wire)
```

### 查询 Memory Node（NWP）

```go
import "github.com/labacacia/nps/impl/go/nwp"

client := nwp.NewClient("http://node.example.com:17433")
caps, err := client.Query(ctx, &nwp.QueryFrame{
    AnchorRef: "sha256:<id>",
    Limit:     50,
})
```

### Ed25519 身份（NIP）

```go
import "github.com/labacacia/nps/impl/go/nip"

id, _ := nip.GenerateIdentity()

// 使用 AES-256-GCM + PBKDF2 口令持久化
_ = id.Save("node.key", "my-passphrase")

// 加载并签名
loaded, _ := nip.LoadIdentity("node.key", "my-passphrase")
sig, _    := loaded.Sign(map[string]any{"nid": "urn:nps:node:example.com:data"})
ok, _     := loaded.Verify(map[string]any{"nid": "urn:nps:node:example.com:data"}, sig)
```

### 公告与解析（NDP）

```go
import "github.com/labacacia/nps/impl/go/ndp"

registry  := ndp.NewInMemoryRegistry()
validator := ndp.NewAnnounceValidator()
validator.RegisterPublicKey(nid, id.PubKeyString())

_ = registry.Announce(frame)
resolved, _ := registry.Resolve("nwp://example.com/data")
```

### 提交 NOP 任务

```go
import "github.com/labacacia/nps/impl/go/nop"

client  := nop.NewClient("http://orchestrator.example.com:17433")
taskID, _ := client.Submit(ctx, &nop.TaskFrame{
    TaskID: "job-1",
    DAG: nop.TaskDAG{
        Nodes: []nop.TaskNode{{ID: "a", Action: "data.fetch", Agent: "urn:nps:node:data.example.com"}},
    },
})
status, _ := client.Wait(ctx, taskID, 30*time.Second)
```

## 编码分层

| Tier | 常量 | 说明 |
|------|------|------|
| Tier-1 | `core.EncodingTierJSON` | UTF-8 JSON —— 开发 / 互操作 |
| Tier-2 | `core.EncodingTierMsgPack` | MessagePack —— 默认，体积约小 60% |

## NIP CA Server

`ca-server/` 目录提供一个独立 NIP 证书颁发机构服务 —— 基于 `net/http` 标准库，SQLite 存储，开箱即用的 Docker 部署。

## 测试

```bash
go test ./...
```

## 许可证

Apache 2.0 —— 详见 [LICENSE](./LICENSE) 与 [NOTICE](./NOTICE)。

Copyright 2026 INNO LOTUS PTY LTD
