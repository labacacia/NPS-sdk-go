[English Version](./CHANGELOG.md) | 中文版

# 变更日志 —— Go SDK (`github.com/labacacia/NPS-sdk-go`)

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

在 NPS 达到 v1.0 稳定版之前，套件内所有仓库同步使用同一个预发布版本号。

---

## [1.0.0-alpha.14] —— 2026-06-26

### Added

- `nip.NipCaClient`：远程 NIP CA 的类型化客户端，覆盖 discovery、CRL、agent/node 注册、X.509 注册、续签、撤销和校验。
- `nwp.NwpNativeNodeServer`：native-mode NWP 服务端 helper，用于在已建立的 NCP stream 上分发 QueryFrame/ActionFrame。
- `conformance`：TC-N1/TC-N2 一致性用例目录、manifest 构造器和校验器，用于 CI/自认证流程。

---

## [1.0.0-alpha.6] —— 2026-05-12

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

[1.0.0-alpha.6]: https://github.com/LabAcacia/nps/releases/tag/v1.0.0-alpha.6
[1.0.0-alpha.2]: https://github.com/LabAcacia/nps/releases/tag/v1.0.0-alpha.2
[1.0.0-alpha.1]: https://github.com/LabAcacia/nps/releases/tag/v1.0.0-alpha.1
