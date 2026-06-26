English | [中文版](./CHANGELOG.cn.md)

# Changelog — Go SDK (`github.com/labacacia/NPS-sdk-go`)

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Until NPS reaches v1.0 stable, every repository in the suite is synchronized to the same pre-release version tag.

---

## [1.0.0-alpha.14] — 2026-06-26

### Added
- `nip.NipCaClient`: typed remote NIP CA client for discovery, CRL, agent/node registration, X.509 registration, renewal, revocation, and verification.
- `nwp.NwpNativeNodeServer`: native-mode NWP serving helper for dispatching QueryFrame/ActionFrame over an already established NCP stream.
- `conformance`: TC-N1/TC-N2 conformance catalog, manifest builder, and validator for CI/self-certification flows.

---

## [1.0.0-alpha.11] — 2026-05-28

### Added
- NOP saga compensation: DagNode.CompensateAction/CompensateParamsMapping, TaskFrame.CompensationPolicy, TaskStateCompensating/Compensated, CompensationPolicy constants (alpha.9 parity)
- NOP AggregateStrategyWeightedFirstK and MergeAll (alpha.11)
- NOP DelegateFrame.TargetClusterAnchor, AlignStreamFrame.AckSeq/NakSeq (alpha.11)
- NDP security profiles: constants + InMemoryRegistry.SecurityProfile enforcement (alpha.9 parity)
- NDP ephemeral TTL cap (60 s) in registry (alpha.9 parity)
- NDP AnnounceFrame alpha.9 fields: NodeRoles, ClusterAnchor, SpawnSpecRef, BridgeProtocols, ActivationMode, ActivationEndpoint
- NDP GraphFrame / GraphEdge redesigned to NPS-4 §5 topology snapshot format (alpha.11)
- NIP IdentReputationPolicyHint and IdentMetadata.ReputationPolicy (alpha.10 parity)
- NIP IdentFrame.OCSPStaple (alpha.11)
- NWP SubscribeFrame and NWM TrustAnchors field (alpha.11)

---

## [1.0.0-alpha.6] — 2026-05-12

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
