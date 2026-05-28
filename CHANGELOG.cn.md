[English Version](./CHANGELOG.md) | 中文版

# 变更日志 —— Go SDK (`github.com/labacacia/NPS-sdk-go`)

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

在 NPS 达到 v1.0 稳定版之前，套件内所有仓库同步使用同一个预发布版本号。

---

## [1.0.0-alpha.8] —— 2026-05-28

### 套件同步

本版本跟进 NPS 套件 `v1.0.0-alpha.8`。

套件 alpha.8 亮点：.NET SDK 落地 RFC-0005 `ReputationPolicyEvaluator`；
cgn_limit 预执行拦截；RFC-0002 与 RFC-0005 晋级为 Accepted。

---

## [1.0.0-alpha.7] —— 2026-05-17

### 新增

- **`nip.ReputationLogClient`（NPS-RFC-0004 Phase 2）**：完整的声誉日志 operator HTTP 客户端。`GetSnapshot`、`QueryEntries`、`GetSTH`、`GetProof`、`GetGossipSTH`。`VerifyInclusion` 在本地执行 RFC 9162 §2.1.3.2 Merkle audit-path 验证。`SignEntry` / `VerifyEntry` 使用 Ed25519 签名和验证条目。Wire 类型：`ReputationLogEntry`、`SignedTreeHead`、`InclusionProof`。`ReputationLogError` 携带 `Code` + `Status`。`nip/error_codes.go` 新增 `ErrReputationGossipFork` 和 `ErrReputationGossipSigInvalid`。20 条回归测试。

- **`nwp.AnchorNodeClient`（NPS-CR-0002）**：Anchor Node 拓扑查询 HTTP 客户端。`GetSnapshot` 和 `Subscribe`（基于 channel 的 NDJSON 流）。类型化事件：`MemberJoinedEvent`、`MemberLeftEvent`、`MemberUpdatedEvent`、`AnchorStateEvent`、`ResyncRequiredEvent`。`AnchorTopologyError` 处理协议错误。`WithPathPrefix`、`WithHTTPClient` 选项。21 条回归测试。

### 跟随套件

本次跟随 NPS 套件 `v1.0.0-alpha.7`。

---

## [1.0.0-alpha.6] —— 2026-05-14

### 变更

- **`nip/x509` — IANA PEN 65715（Breaking，CR-0004）**：所有 OID 常量现在使用已分配的弧 `1.3.6.1.4.1.65715`（替换临时弧 `1.3.6.1.4.1.99999`）。新增 `OidIdNpsNodeRoles`（`1.3.6.1.4.1.65715.2.2`），按 CR-0004 预留，暂无消费者。在临时弧下签发的证书必须吊销并重新签发。

- **`nip/error_codes.go` — 移除 `ErrReputationGossipFork` / `ErrReputationGossipSigInvalid`**：这两个常量是提前加入的 Phase 3 条目，在 RFC-0004 Phase 3 完整规范落地前从 alpha.6 中移除。

- **版本升级至 `v1.0.0-alpha.6`** —— 与 NPS 套件 alpha.6 版本同步。

---

## [1.0.0-alpha.5] —— 2026-05-01

### 新增

- **`nwp.ErrAuth*` / `ErrQuery*` / `ErrAction*` / `ErrTask*` / `ErrSubscribe*` / `ErrManifest*` / `ErrTopology*` / `ErrReservedTypeUnsupported`** —— 新增 `nwp/error_codes.go`，包含全部 30 个 NWP wire 错误码常量。此前版本均未提供。
- **`ndp.ResolveViaDns` —— DNS TXT 回退解析** —— 新增 `(*InMemoryNdpRegistry).ResolveViaDns(ctx, target, lookup)`，当内存注册表无匹配时回退查询 `_nps-node.{host}` TXT 记录（NPS-4 §5）。`DnsTxtLookup` 接口 + `SystemDnsTxtLookup`（`net.DefaultResolver`）；辅助函数 `ParseNpsTxtRecord` + `ExtractHostFromTarget` 位于 `ndp/dns_txt.go`。测试数：96 → 106。

### 修复

- **`nip.AssuranceFromWire("")` 现在返回 `AssuranceAnonymous`** —— `AssuranceFromWire` 此前缺少空字符串判断。修复新增显式 `if wire == ""` 分支，返回 `AssuranceAnonymous, nil`（NPS-RFC-0003 §5.1.1 向后兼容）。
- **`nip.ErrReputationGossipFork` / `ErrReputationGossipSigInvalid`** —— 向 `nip/error_codes.go` 新增两个 NIP 声誉 gossip 错误码（RFC-0004 Phase 3）。

### 变更

- **版本升至 `v1.0.0-alpha.5`** —— `README.md` / `README.cn.md` 已更新；与 NPS 套件 alpha.5 同步。
- **套件合并跟踪** —— alpha.5.2 仅用于跟踪的子版本已合回 alpha.5（refs #28）。按"不允许逐包子版本"策略，子补丁号统一回收，内容并入父套件版本。

---

## [1.0.0-alpha.4] —— 2026-04-30

### 新增

- **NPS-RFC-0001 Phase 2 —— NCP 连接前导（Go helper 跟进）。**
  `ncp/preamble.go` 暴露 `WritePreamble(io.Writer)` 与
  `ReadPreamble(io.Reader)`，往返字面量 `b"NPS/1.0\n"` 哨兵；
  对应测试在 `ncp/preamble_test.go`。让 Go SDK 与 .NET / Python /
  TypeScript / Java 在 alpha.4 的 preamble helper 持平。
- **NPS-RFC-0002 Phase A/B —— X.509 NID 证书 + ACME `agent-01`
  （Go 端口）。** 新增 `nip/` 子模块：
  - `nip/x509/` —— X.509 NID 证书 builder + verifier
    (`x509.Builder`, `x509.Verifier`)，复用 Go 标准库 `crypto/x509`。
  - `nip/acme/` —— ACME `agent-01` 客户端 + 服务端参考实现（挑战签发、
    key authorization、按 NPS-RFC-0002 Phase B 的 JWS 签名 wire 包络）。
  - `nip/assurance_level.go` —— Agent 身份保证等级
    （`anonymous` / `attested` / `verified`），承接 NPS-RFC-0003。
  - `nip/cert_format.go` —— IdentFrame 的 `cert_format` 判别器
    （`v1` Ed25519 vs. `x509`）。
  - `nip/error_codes.go` —— NIP 错误码命名空间。
  - `nip/verifier.go` —— dual-trust IdentFrame 验证器（v1 + X.509）。
- 21 个新测试覆盖 preamble 往返、X.509 签发 + 解析、dual-trust 验证、
  ACME agent-01 全流程。总数：96 tests 全绿（alpha.3 时为 75）。

### 变更

- 版本升级至 `1.0.0-alpha.4`，与 NPS `v1.0.0-alpha.4` 套件同步。
- `nip/frames.go` —— IdentFrame wire 形状扩展，可携带可选的
  `cert_format` 判别器 + 叶子 `x509_chain` 字段，与 v1 Ed25519
  字段并存。alpha.3 写出的 v1 IdentFrame 仍可被 alpha.4 验签。

### 套件级 alpha.4 要点

- **NPS-RFC-0002 X.509 + ACME** —— 完整跨 SDK 端口波（.NET / Java /
  Python / TypeScript / Go / Rust）。服务端可签发 dual-trust IdentFrame
  （v1 Ed25519 + X.509 leaf 链回自签 root），NID 可通过 ACME
  `agent-01` 自助上线。
- **NPS-CR-0002 —— Anchor Node topology 查询** ——
  `topology.snapshot` / `topology.stream` 查询类型（.NET 参考实现 +
  L2 conformance 测试套）。Go 消费侧 helper 后续版本跟进；本 SDK
  的 `nwp.AnchorNode` 形状与 alpha.3 一致。
- **`nps-registry` SQLite 实仓** + **`nps-ledger` Phase 2**（SQLite +
  Merkle + STH + inclusion proof）在对应 daemon 仓库交付。

### 涵盖模块

- core / ncp / nwp / nip（新增 `nip/x509`、`nip/acme`） / ndp / nop

---

## [1.0.0-alpha.3] —— 2026-04-25

### Changed

- 将 Go SDK 源码与包元数据同步到套件统一版本 `1.0.0-alpha.6`。
- 对齐 NIP error/OID 常量，并移除当前 SDK API 不再暴露的独立 NWP error-code 表面。

---

## [1.0.0-alpha.2] —— 2026-04-19

### Changed

- 版本升级至 `1.0.0-alpha.2`，与套件同步。除版本对齐外无功能变更。
- 75 tests 全绿。

### 涵盖模块

- core / ncp / nwp / nip / ndp / nop

---

## [1.0.0-alpha.1] —— 2026-04-10

作为 NPS 套件 `v1.0.0-alpha.1` 的一部分首次公开 alpha。

[1.0.0-alpha.7]: https://github.com/labacacia/NPS-sdk-go/releases/tag/v1.0.0-alpha.7
[1.0.0-alpha.6]: https://github.com/LabAcacia/nps/releases/tag/v1.0.0-alpha.6
[1.0.0-alpha.2]: https://github.com/LabAcacia/nps/releases/tag/v1.0.0-alpha.2
[1.0.0-alpha.1]: https://github.com/LabAcacia/nps/releases/tag/v1.0.0-alpha.1
