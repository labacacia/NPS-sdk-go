English | [中文版](./CHANGELOG.cn.md)

# Changelog — Go SDK (`github.com/labacacia/NPS-sdk-go`)

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Until NPS reaches v1.0 stable, every repository in the suite is synchronized to the same pre-release version tag.

---

## [1.0.0-alpha.6] — 2026-05-14

### Changed

- **`nip/x509` — IANA PEN 65715 (Breaking, CR-0004)**: All OID constants now use the assigned arc `1.3.6.1.4.1.65715` (replacing provisional `1.3.6.1.4.1.99999`). New `OidIdNpsNodeRoles` (`1.3.6.1.4.1.65715.2.2`) reserved per CR-0004. Certificates issued under the provisional arc must be revoked and re-issued.

- **`nip/error_codes.go` — `ErrReputationGossipFork` / `ErrReputationGossipSigInvalid` removed**: These two constants were premature Phase 3 additions; removed pending the RFC-0004 Phase 3 full specification.

- **Version bump to `v1.0.0-alpha.6`** — synchronized with NPS suite alpha.6 release.

---

## [1.0.0-alpha.5] — 2026-05-01

### Added

- **`nwp.ErrAuth*` / `ErrQuery*` / `ErrAction*` / `ErrTask*` / `ErrSubscribe*` / `ErrManifest*` / `ErrTopology*` / `ErrReservedTypeUnsupported`** — new `nwp/error_codes.go` with all 30 NWP wire error code constants. Missing from previous releases.
- **`ndp.ResolveViaDns` — DNS TXT fallback resolution** — new `(*InMemoryNdpRegistry).ResolveViaDns(ctx, target, lookup)` falls back to `_nps-node.{host}` TXT lookup (NPS-4 §5) when no in-memory entry matches. `DnsTxtLookup` interface + `SystemDnsTxtLookup` (`net.DefaultResolver`); `ParseNpsTxtRecord` + `ExtractHostFromTarget` helpers in `ndp/dns_txt.go`. Tests: 96 → 106.

### Fixed

- **`nip.AssuranceFromWire("")` now returns `AssuranceAnonymous`** — `AssuranceFromWire` previously had no empty-string guard. Fix adds an explicit `if wire == ""` branch returning `AssuranceAnonymous, nil` (NPS-RFC-0003 §5.1.1 backward compat).
- **`nip.ErrReputationGossipFork` / `ErrReputationGossipSigInvalid`** — two new NIP reputation gossip error codes added to `nip/error_codes.go` (RFC-0004 Phase 3).

### Changed

- Synchronized Go SDK source and package metadata with the suite-wide `1.0.0-alpha.6` release.
- Aligned NIP error/OID constants and removed the standalone NWP error-code surface that is no longer part of the active SDK API.

---

## [1.0.0-alpha.2] — 2026-04-19

### Changed

- Version bump to `1.0.0-alpha.2` for suite-wide synchronization. No functional changes beyond version alignment.
- 75 tests green.

### Covered modules

- core / ncp / nwp / nip / ndp / nop

---

## [1.0.0-alpha.1] — 2026-04-10

First public alpha as part of the NPS suite `v1.0.0-alpha.1` release.

[1.0.0-alpha.6]: https://github.com/LabAcacia/nps/releases/tag/v1.0.0-alpha.6
[1.0.0-alpha.2]: https://github.com/LabAcacia/nps/releases/tag/v1.0.0-alpha.2
[1.0.0-alpha.1]: https://github.com/LabAcacia/nps/releases/tag/v1.0.0-alpha.1
