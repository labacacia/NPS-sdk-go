[English Version](./README.md) | 中文版

# NPS Go SDK v1.0.0-alpha.4

Neural Protocol Suite (NPS) 的 Go 参考实现 —— 覆盖五个子协议：**NCP · NWP · NIP · NDP · NOP**，外加完整的 NPS-RFC-0002 X.509 + ACME `agent-01` NID 证书原语。

| | |
|---|---|
| **Module** | `github.com/labacacia/NPS-sdk-go` |
| **Go** | 1.25+ |
| **测试** | 86 通过 |
| **许可证** | Apache 2.0 |

---

## 包

| Package | 协议 | 说明 |
|---------|------|------|
| `core` | NCP | 帧类型、帧头编解码、注册表、AnchorFrame 缓存 |
| `ncp` | NCP | AnchorFrame、DiffFrame、StreamFrame、CapsFrame、HelloFrame、ErrorFrame |
| `nwp` | NWP | QueryFrame、ActionFrame、NwpClient（HTTP 模式） |
| `nip` | NIP | IdentFrame（v2 双信任）、TrustFrame、RevokeFrame、NipIdentity（Ed25519）、NipIdentVerifier（RFC-0002 §8.1 双信任）、AssuranceLevel（RFC-0003） |
| `nip/x509` | NIP / RFC-0002 | `IssueLeaf` / `IssueRoot` / `Verify` —— 基于 stdlib `crypto/x509` 的 NPS X.509 NID 证书 |
| `nip/acme` | NIP / RFC-0002 | `Client` + `Server`（进程内） + JWS / messages —— ACME `agent-01` 流程 |
| `ndp` | NDP | AnnounceFrame、ResolveFrame、GraphFrame、InMemoryNdpRegistry、NdpAnnounceValidator |
| `nop` | NOP | TaskFrame、DelegateFrame、SyncFrame、AlignStreamFrame、NopClient |

---

## 安装

```bash
go get github.com/labacacia/NPS-sdk-go
```

---

## 快速开始

### NCP —— 帧编解码

```go
import (
    "github.com/labacacia/NPS-sdk-go/core"
    "github.com/labacacia/NPS-sdk-go/ncp"
)

// 创建完整注册表（5 个协议）
reg := core.CreateFullRegistry()
codec := core.NewNpsFrameCodec(reg)

// 构造并编码 AnchorFrame
frame := &ncp.AnchorFrame{
    AnchorID: "sha256:abc...",
    Schema:   core.FrameDict{"type": "object", "version": "1"},
    TTL:      3600,
}
wire, err := codec.Encode(frame.FrameType(), frame.ToDict(), core.EncodingTierMsgPack, true)
// wire 已可发送

// 接收端解码
ft, dict, err := codec.Decode(wire)
received := ncp.AnchorFrameFromDict(dict)
```

### NCP —— AnchorFrame 缓存

```go
cache := core.NewAnchorFrameCache()

// 带 TTL 存储
schema := core.FrameDict{"type": "object", "fields": []any{"name", "value"}}
anchorID, err := cache.Set(schema, 3600) // 1 小时 TTL

// 读取（过期返回 nil）
schema, err = cache.GetRequired(anchorID)
```

### NWP —— HTTP 客户端

```go
import "github.com/labacacia/NPS-sdk-go/nwp"

client := nwp.NewNwpClient("http://node.example.com:17433")

// 查询
qf := &nwp.QueryFrame{AnchorRef: "sha256:abc...", Filters: map[string]any{"status": "active"}}
capsFrame, err := client.Query(ctx, qf)

// 流式
frames, err := client.Stream(ctx, qf)
for _, sf := range frames {
    fmt.Println(sf.Payload)
}

// 同步 Action 调用
af := &nwp.ActionFrame{Action: "create", Payload: map[string]any{"name": "item"}}
result, err := client.Invoke(ctx, af)

// 异步 Action 调用
af.Async = true
result, err = client.Invoke(ctx, af)
fmt.Println(result.Async.TaskID)
```

### NIP —— 身份与签名

```go
import "github.com/labacacia/NPS-sdk-go/nip"

// 生成新身份
id, err := nip.Generate()
fmt.Println(id.PubKeyString()) // "ed25519:<hex>"

// 对帧 dict 签名
payload := core.FrameDict{"nid": "urn:nps:node:example.com:agent", "pub_key": id.PubKeyString()}
sig := id.Sign(payload)

// 验签
ok := id.Verify(payload, sig)

// 仅凭公钥字符串验签（无需私钥）
ok = nip.VerifyWithPubKeyStr(payload, "ed25519:<hex>", sig)

// 保存 / 加载（AES-256-GCM + PBKDF2-SHA256，600k 轮）
err = id.Save("/path/to/identity.json", "my-passphrase")
loaded, err := nip.Load("/path/to/identity.json", "my-passphrase")
```

### NDP —— 发现注册表

```go
import "github.com/labacacia/NPS-sdk-go/ndp"

registry := ndp.NewInMemoryNdpRegistry()

// 公告节点
frame := &ndp.AnnounceFrame{
    NID:       "urn:nps:node:example.com:agent",
    Addresses: []map[string]any{{"host": "example.com", "port": uint64(17433), "protocol": "nps"}},
    Caps:      []string{"nwp", "nop"},
    TTL:       300,
    Timestamp: time.Now().UTC().Format(time.RFC3339),
}
registry.Announce(frame)

// 解析目标 URL
result := registry.Resolve("nwp://example.com/agent")
// result.Host、result.Port、result.Protocol

// 校验 announce 签名
validator := ndp.NewNdpAnnounceValidator()
validator.RegisterPublicKey("urn:nps:node:example.com:agent", "ed25519:<hex>")
frame.Signature = id.Sign(frame.UnsignedDict())
result := validator.Validate(frame)
// result.IsValid、result.ErrorCode、result.Message
```

### NOP —— 编排客户端

```go
import "github.com/labacacia/NPS-sdk-go/nop"

client := nop.NewNopClient("http://orchestrator.example.com:17433")

// 提交 DAG 任务
tf := &nop.TaskFrame{
    TaskID:    "task-" + uuid,
    DAG:       map[string]any{...},
    TimeoutMs: &timeout,
}
taskID, err := client.Submit(ctx, tf)

// 轮询状态
status, err := client.GetStatus(ctx, taskID)
fmt.Println(status.State()) // "running"

// 等待完成（每 500ms 轮询一次）
status, err = client.Wait(ctx, taskID, nil)
fmt.Println(status.State())        // "completed"
fmt.Println(status.NodeResults())  // map[string]any

// 取消
err = client.Cancel(ctx, taskID)
```

---

## 帧类型

| 帧 | 类型码 | 包 |
|----|--------|----|
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

## 编码

| Tier | 常量 | 备注 |
|------|------|------|
| JSON | `core.EncodingTierJSON` | 可读，Tier-1 |
| MsgPack | `core.EncodingTierMsgPack` | 约 60% 体积削减，Tier-2（默认） |

---

## 退避策略（NOP）

```go
delay := nop.ComputeDelayMs(nop.BackoffExponential, 100, 5000, attempt)
// BackoffFixed、BackoffLinear、BackoffExponential
```

---

## 运行测试

```bash
go test ./...
```

---

## 许可证

Apache 2.0 —— Copyright 2026 INNO LOTUS PTY LTD
