[English Version](./CHANGELOG.md) | 中文版

# 变更日志 —— Go SDK (`github.com/labacacia/NPS-sdk-go`)

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

在 NPS 达到 v1.0 稳定版之前，套件内所有仓库同步使用同一个预发布版本号。

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

- 版本升级至 `1.0.0-alpha.3`，与 NPS `v1.0.0-alpha.3` 套件同步。本次 Go SDK 无功能变更（Go module 通过 git tag 自动发现版本，无 `version` 字段需更新）。
- 75 tests 仍全绿。

### 套件级 alpha.3 要点（各语言 helper 在 alpha.4 跟进）

- **NPS-RFC-0001 —— NCP 连接前导**（Accepted）。原生模式连接现以字面量 `b"NPS/1.0\n"`（8 字节）开头。.NET SDK 已落地参考实现；Go helper 在 alpha.4 跟进。
- **NPS-RFC-0003 —— Agent 身份保证等级**（Accepted）。NIP IdentFrame 与 NWM 新增三态 `assurance_level`（`anonymous`/`attested`/`verified`）。.NET 参考类型已落地；Go 同步在 alpha.4。
- **NPS-RFC-0004 —— NID 声誉日志（CT 风格）**（Accepted）。append-only Merkle 日志条目结构发布；.NET 参考签名器已落地（并以 `nps-ledger` daemon Phase 1 形态发布）。Go helper 在 alpha.4 跟进。
- **NPS-CR-0001 —— Anchor / Bridge 节点拆分。** 旧的 "Gateway Node" 角色更名为 **Anchor Node**；"NPS↔外部协议翻译" 单独成为 **Bridge Node** 类型。AnnounceFrame 新增 `node_kind` / `cluster_anchor` / `bridge_protocols`。源代码层面变更落在 `spec/` + .NET 参考实现。
- **6 个 NPS 常驻 daemon。** NPS-Dev 新建 `daemons/` 目录，定义 `npsd` / `nps-runner` / `nps-gateway` / `nps-registry` / `nps-cloud-ca` / `nps-ledger`；其中 `npsd` 提供 L1 功能性参考实现，其余为 Phase 1 骨架。

### 涵盖模块

- core / ncp / nwp / nip / ndp / nop

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

[1.0.0-alpha.4]: https://gitee.com/labacacia/NPS-sdk-go/releases/tag/v1.0.0-alpha.4
[1.0.0-alpha.3]: https://github.com/LabAcacia/NPS-Dev/releases/tag/v1.0.0-alpha.3
[1.0.0-alpha.2]: https://github.com/LabAcacia/NPS-Dev/releases/tag/v1.0.0-alpha.2
[1.0.0-alpha.1]: https://github.com/LabAcacia/NPS-Dev/releases/tag/v1.0.0-alpha.1
