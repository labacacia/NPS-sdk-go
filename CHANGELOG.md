English | [中文版](./CHANGELOG.cn.md)

# Changelog — Go SDK (`github.com/labacacia/NPS-sdk-go`)

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Until NPS reaches v1.0 stable, every repository in the suite is synchronized to the same pre-release version tag.

---

## [1.0.0-alpha.12] — 2026-06-03

### Added

- **NCP v0.8 — `NopFrame` (0x07)**: `FrameTypeNop = 0x07`; `NopFrame` struct; `HelloFrame.PingIntervalMs` (`uint64`, default `0`); `ErrKeepaliveTimeout` / `ErrRekeyRequired` error codes.
- **NWP v0.14 — manifest versioning**: `XNwmVersion = "X-NWM-Version"` header constant.
- **NIP v0.10 — `node_roles`**: `IdentFrame.NodeRoles` (`[]string`); `ErrCertNodeRolesMismatch` error code.
- **NDP v0.9 — spawn schema + heartbeat**: `AnnounceFrame.SpawnSpecRef` changed to `map[string]any`; `AnnounceFrame.HeartbeatIntervalMs` (`uint64`, default `60000`); `ErrAnnounceStale` error code.
- **NOP v0.7 — result TTL**: `TaskFrame.ResultTtlSeconds` (`uint64`, default `3600`); `ErrTaskResultExpired` / `ErrStreamNakUnresolvable` error codes.

### Tracking the suite

This release tracks NPS suite `v1.0.0-alpha.12`. NCP v0.8 / NWP v0.14 / NIP v0.10 / NDP v0.9 / NOP v0.7.

---

## [1.0.0-alpha.11] — 2026-05-31

### Added

- **NWP — `SubscribeFrame` CR-0006** (Breaking rewrite): Wire format replaced with CR-0006 formal spec — `SubscriptionID` (UUID v4), `Filter` (`map[string]any?`), `HeartbeatIntervalMs` (`*uint32`), `MaxEvents` (`*uint32`), `Cursor` (`*string`). **Wire breaking change vs alpha.8–10.**
- **NOP — AlignStream ack/NAK**: `AlignStreamFrame` gains `AckSeq` and `NakSeq` (`*uint64`) for NOP v0.6 sliding-window acknowledgement.
- **NOP — Saga compensation**: `TaskFrame.CompensationPolicy`; `DelegateFrame.TargetClusterAnchor` for cross-cluster routing; `AggregateStrategy` constants `WeightedFirstK` / `MergeAll`.
- **NDP — GraphFrame §5** (Breaking rewrite): `GraphNode`, `GraphEdge` structs; `GraphFrame` with `GraphID`, `Nodes`, `Edges`, `TTL`, `Metadata`. Max 256 nodes / 1024 edges.
- **NIP — `IdentFrame.OCSPStaple`**: base64url DER OCSP response field; `IdentReputationPolicyHint` struct.

### Tracking the suite

This release tracks NPS suite `v1.0.0-alpha.11`. NCP v0.7 / NWP v0.13 / NIP v0.9 / NDP v0.8 / NOP v0.6.

---

## [1.0.0-alpha.10] — 2026-05-28

### Added

- **NOP — Saga compensation**: `DagNode` with `CompensateAction` / `CompensateParamsMapping`; `TaskStateCompensating` / `TaskStateCompensated` constants; `CompensationPolicy` constants.
- **NDP — `SecurityProfile`**: `SecurityProfileLocalDev` / `SecurityProfileOrgPrivate` / `SecurityProfilePublicFederated` constants.
- **NIP — `IdentReputationPolicyHint`**: Reputation policy hint struct for identity frames.

### Tracking the suite

This release tracks NPS suite `v1.0.0-alpha.10`.

---

## [1.0.0-alpha.9] — 2026-05-28

### Added

- **NWP — `SubscribeFrame` (0x12)**: Initial `SubscribeFrame` struct (pre-CR-0006 format — replaced in alpha.11).
- **NIP — `IdentFrame` assurance improvements**: Structured assurance-level extraction aligned with NPS-RFC-0003 draft.

### Tracking the suite

This release tracks NPS suite `v1.0.0-alpha.9`.

---

## [1.0.0-alpha.8] — 2026-05-28

### Tracking the suite

This release tracks NPS suite `v1.0.0-alpha.8`.

Suite highlights: RFC-0005 `ReputationPolicyEvaluator` in .NET SDK; cgn_limit
pre-execution enforcement; RFC-0002 and RFC-0005 promoted to Accepted.

---

## [1.0.0-alpha.7] — 2026-05-17

### Added

- **`nip.ReputationLogClient` (NPS-RFC-0004 Phase 2)**: Full HTTP client for the reputation-log operator API. `GetSnapshot`, `QueryEntries`, `GetSTH`, `GetProof`, `GetGossipSTH`. `VerifyInclusion` performs RFC 9162 §2.1.3.2 Merkle audit-path verification locally. `SignEntry` / `VerifyEntry` sign and verify entries with Ed25519. Wire types: `ReputationLogEntry`, `SignedTreeHead`, `InclusionProof`. `ReputationLogError` carries `Code` + `Status`. Error codes `ErrReputationGossipFork` and `ErrReputationGossipSigInvalid` added to `nip/error_codes.go`. 20 regression tests.

- **`nwp.AnchorNodeClient` (NPS-CR-0002)**: HTTP client for Anchor Node topology queries. `GetSnapshot` and `Subscribe` (channel-based NDJSON streaming). Typed events: `MemberJoinedEvent`, `MemberLeftEvent`, `MemberUpdatedEvent`, `AnchorStateEvent`, `ResyncRequiredEvent`. `AnchorTopologyError` for protocol errors. `WithPathPrefix`, `WithHTTPClient` options. 21 regression tests.

### Tracking the suite

This release tracks NPS suite `v1.0.0-alpha.7`.

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

[1.0.0-alpha.7]: https://github.com/labacacia/NPS-sdk-go/releases/tag/v1.0.0-alpha.7
[1.0.0-alpha.6]: https://github.com/LabAcacia/nps/releases/tag/v1.0.0-alpha.6
[1.0.0-alpha.2]: https://github.com/LabAcacia/nps/releases/tag/v1.0.0-alpha.2
[1.0.0-alpha.1]: https://github.com/LabAcacia/nps/releases/tag/v1.0.0-alpha.1
