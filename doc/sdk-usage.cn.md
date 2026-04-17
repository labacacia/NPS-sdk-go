[English Version](./sdk-usage.md) | 中文版

# NPS Go SDK — 使用指南

> Copyright 2026 INNO LOTUS PTY LTD — 基于 Apache 2.0 授权

**版本**: v1.0.0-alpha.1 | **模块**: `github.com/labacacia/nps/impl/go` | **Go**: 1.25+

---

## 目录

- [安装](#安装)
- [包说明](#包说明)
- [快速开始](#快速开始)
- [API 参考](#api-参考)
- [测试](#测试)

---

## 安装

```bash
go get github.com/labacacia/nps/impl/go
```

**依赖项**

| 依赖 | 用途 |
|---|---|
| `github.com/vmihailenco/msgpack/v5` | Tier-2 MsgPack 编码 |
| `golang.org/x/crypto` | Ed25519 密钥派生（PBKDF2） |

---

## 包说明

| 包名 | 协议 | 描述 |
|---|---|---|
| `core` | NCP 基础 | 帧类型、帧头编解码、帧注册表、AnchorFrame 缓存 |
| `ncp` | NCP | AnchorFrame、DiffFrame、StreamFrame、CapsFrame、ErrorFrame |
| `nwp` | NWP | QueryFrame、ActionFrame、NwpClient（HTTP 模式） |
| `nip` | NIP | IdentFrame、TrustFrame、RevokeFrame、NipIdentity（Ed25519 + AES-256-GCM） |
| `ndp` | NDP | AnnounceFrame、ResolveFrame、GraphFrame、InMemoryNdpRegistry、NdpAnnounceValidator |
| `nop` | NOP | TaskFrame、DelegateFrame、SyncFrame、AlignStreamFrame、NopClient |

---

## 快速开始

### NCP — 帧编解码

```go
import (
    "github.com/labacacia/nps/impl/go/core"
    "github.com/labacacia/nps/impl/go/ncp"
)

// 构建 AnchorFrame
frame := &ncp.AnchorFrame{
    AnchorID: "anchor-001",
    Schema:   core.FrameDict{"fields": []string{"id", "name", "score"}},
    TTL:      3600,
}

// 使用 MsgPack 编码（Tier-2，生产推荐）
codec := core.NewNpsFrameCodec(core.CreateFullRegistry())
wire, err := codec.Encode(frame.FrameType(), frame.ToDict(), core.EncodingTierMsgPack, true)
if err != nil {
    log.Fatal(err)
}

// 解码
ft, dict, err := codec.Decode(wire)
if err != nil {
    log.Fatal(err)
}
decoded := ncp.AnchorFrameFromDict(dict)
fmt.Println(ft, decoded.AnchorID) // 0x01 anchor-001
```

### NWP — HTTP 模式客户端

```go
import (
    "context"
    "github.com/labacacia/nps/impl/go/nwp"
)

client := nwp.NewNwpClient("http://localhost:17433")

// 发送 AnchorFrame 到 /anchor
anchor := &ncp.AnchorFrame{
    AnchorID: "schema-v1",
    Schema:   core.FrameDict{"type": "agent-profile"},
    TTL:      3600,
}
if err := client.SendAnchor(ctx, anchor); err != nil {
    log.Fatal(err)
}

// 查询：POST /query -> CapsFrame
qf := &nwp.QueryFrame{
    QueryID:  "q-001",
    AnchorID: "schema-v1",
}
caps, err := client.Query(context.Background(), qf)
if err != nil {
    log.Fatal(err)
}
fmt.Println(caps.Caps)

// 调用动作：POST /invoke
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

### NIP — 身份标识与签名

```go
import (
    "github.com/labacacia/nps/impl/go/nip"
    "github.com/labacacia/nps/impl/go/core"
)

// 生成新的 Ed25519 身份标识
id, err := nip.Generate()
if err != nil {
    log.Fatal(err)
}
fmt.Println(id.PubKeyString()) // "ed25519:<hex>"

// 对 payload 签名
payload := core.FrameDict{"nid": "agent-001", "action": "register"}
sig := id.Sign(payload)
fmt.Println(sig) // "ed25519:<base64>"

// 验证签名
ok := id.Verify(payload, sig)
fmt.Println(ok) // true

// 使用 AES-256-GCM + PBKDF2 加密持久化密钥
err = id.SaveToFile("/data/agent.key.enc", "my-secret-passphrase")
if err != nil {
    log.Fatal(err)
}

// 从文件加载密钥
id2, err := nip.LoadFromFile("/data/agent.key.enc", "my-secret-passphrase")
if err != nil {
    log.Fatal(err)
}

// 构建 IdentFrame
ident := &nip.IdentFrame{
    NID:       "agent-001",
    PubKey:    id.PubKeyString(),
    Algorithm: "ed25519",
}
fmt.Println(ident.FrameType()) // 0x20
```

### NDP — 服务发现

```go
import (
    "github.com/labacacia/nps/impl/go/ndp"
)

// 带 TTL 淘汰的内存注册表
registry := ndp.NewInMemoryNdpRegistry()

// 发布节点
announce := &ndp.AnnounceFrame{
    NID:      "node-001",
    Host:     "10.0.0.5",
    Port:     17433,
    Protocol: "nwp",
    TTL:      300,
}
registry.Announce(announce)

// 按 NID 解析节点
result := registry.Resolve("node-001")
if result != nil {
    fmt.Printf("host=%s port=%d\n", result.Host, result.Port)
}

// 验证 AnnounceFrame
validator := ndp.NewNdpAnnounceValidator()
if err := validator.Validate(announce); err != nil {
    log.Fatal(err)
}
```

### NOP — 编排客户端

```go
import (
    "context"
    "github.com/labacacia/nps/impl/go/nop"
)

client := nop.NewNopClient("http://localhost:17433")

// 提交任务
task := &nop.TaskFrame{
    TaskID:   "task-001",
    AnchorID: "schema-v1",
    Method:   "summarise",
    Params:   map[string]any{"text": "Neural Protocol Suite 概述"},
}
taskID, err := client.Submit(context.Background(), task)
if err != nil {
    log.Fatal(err)
}

// 轮询任务状态
status, err := client.GetStatus(context.Background(), taskID)
if err != nil {
    log.Fatal(err)
}
fmt.Println(status.Status, status.Result)
```

---

## API 参考

### `core` 包

| 符号 | 描述 |
|---|---|
| `FrameType` | 帧类型常量（0x01–0xFE） |
| `EncodingTierJSON` / `EncodingTierMsgPack` | 编码 Tier-1（JSON）/ Tier-2（MsgPack） |
| `FrameDict` | `map[string]any` — 帧载荷类型 |
| `NpsFrameCodec` | NPS 线格式编解码器 |
| `NewNpsFrameCodec(reg)` | 用帧注册表创建编解码器 |
| `CreateFullRegistry()` | 包含全部 17 种已知帧类型的注册表 |
| `ParseFrameHeader(buf)` | 解析 6 字节或 10 字节帧头 |
| `InMemoryAnchorCache` | 基于 TTL 的 AnchorFrame 缓存 |

### `ncp` 包

| 符号 | 描述 |
|---|---|
| `AnchorFrame` | Schema 声明帧（0x01） |
| `DiffFrame` | Schema 差分帧（0x02） |
| `StreamFrame` | 流式载荷帧（0x03） |
| `CapsFrame` | 能力响应帧（0x04） |
| `ErrorFrame` | 统一错误帧（0xFE） |
| `*FromDict(d)` | 从 `FrameDict` 反序列化各帧类型 |

### `nwp` 包

| 符号 | 描述 |
|---|---|
| `QueryFrame` | HTTP 查询帧（0x10） |
| `ActionFrame` | HTTP 动作调用帧（0x11） |
| `NwpClient` | HTTP 模式 NWP 客户端 |
| `NewNwpClient(baseURL)` | 使用 MsgPack + 默认 HTTP 创建客户端 |
| `client.SendAnchor(ctx, frame)` | POST anchor 到 `/anchor` |
| `client.Query(ctx, frame)` | POST 到 `/query`，返回 `CapsFrame` |
| `client.Stream(ctx, frame)` | POST 到 `/stream`，返回 `[]StreamFrame` |
| `client.Invoke(ctx, frame)` | POST 到 `/invoke`，返回 `InvokeResult` |

### `nip` 包

| 符号 | 描述 |
|---|---|
| `NipIdentity` | Ed25519 签名身份 |
| `Generate()` | 创建新的随机身份 |
| `LoadFromFile(path, passphrase)` | 加载 AES-256-GCM 加密的密钥 |
| `id.SaveToFile(path, passphrase)` | 用 AES-256-GCM + PBKDF2 保存密钥 |
| `id.Sign(payload)` | 对 `FrameDict` 签名，返回 `"ed25519:<base64>"` |
| `id.Verify(payload, sig)` | 验证签名 |
| `id.PubKeyString()` | 返回 `"ed25519:<hex>"` |
| `IdentFrame` | 身份声明帧（0x20） |
| `TrustFrame` | 信任证明帧（0x21） |
| `RevokeFrame` | 吊销帧（0x22） |

### `ndp` 包

| 符号 | 描述 |
|---|---|
| `AnnounceFrame` | 服务发布帧（0x30） |
| `ResolveFrame` | 解析请求帧（0x31） |
| `GraphFrame` | 拓扑图帧（0x32） |
| `InMemoryNdpRegistry` | 带 TTL 淘汰的内存注册表 |
| `registry.Announce(frame)` | 存储或移除（ttl=0）一条记录 |
| `registry.Resolve(nid)` | 将 NID 解析为 `ResolveResult` |
| `NdpAnnounceValidator` | 验证 AnnounceFrame 字段 |

### `nop` 包

| 符号 | 描述 |
|---|---|
| `TaskFrame` | 编排任务帧（0x40） |
| `DelegateFrame` | 委托帧（0x41） |
| `SyncFrame` | 同步帧（0x42） |
| `AlignStreamFrame` | 对齐流帧（0x43） |
| `NopClient` | HTTP 模式 NOP 客户端 |
| `NewNopClient(baseURL)` | 使用默认 HTTP 创建客户端 |
| `client.Submit(ctx, frame)` | POST 任务到 `/tasks`，返回任务 ID |
| `client.GetStatus(ctx, taskID)` | GET `/tasks/{id}`，返回 `NopTaskStatus` |

---

## 测试

```bash
# 运行全部测试
go test ./...

# 详细输出
go test -v ./...

# 运行指定包的测试
go test github.com/labacacia/nps/impl/go/ncp -v

# 开启竞态检测
go test -race ./...
```

SDK 包含 **75 个测试**，覆盖全部 5 个协议包。

---

*默认端口：**17433**（全 NPS 协议族共用）*
