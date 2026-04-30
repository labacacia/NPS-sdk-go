English | [中文版](./CHANGELOG.cn.md)

# Changelog — Go SDK (`github.com/labacacia/NPS-sdk-go`)

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Until NPS reaches v1.0 stable, every repository in the suite is synchronized to the same pre-release version tag.

---

## [1.0.0-alpha.4] — 2026-04-30

### Added

- **NPS-RFC-0001 Phase 2 — NCP connection preamble (Go helper parity).**
  `ncp/preamble.go` exposes `WritePreamble(io.Writer)` and
  `ReadPreamble(io.Reader)` that round-trip the literal `b"NPS/1.0\n"`
  sentinel; matched by `ncp/preamble_test.go`. Brings Go in line with
  the .NET / Python / TypeScript / Java preamble helpers shipped at
  alpha.4.
- **NPS-RFC-0002 Phase A/B — X.509 NID certificates + ACME `agent-01`
  (Go port).** New surface under `nip/`:
  - `nip/x509/` — X.509 NID certificate builder + verifier
    (`x509.Builder`, `x509.Verifier`); reuses Go stdlib `crypto/x509`.
  - `nip/acme/` — ACME `agent-01` client + server reference (challenge
    issuance, key authorization, JWS-signed wire envelope per
    NPS-RFC-0002 Phase B).
  - `nip/assurance_level.go` — agent identity assurance levels
    (`anonymous` / `attested` / `verified`) per NPS-RFC-0003.
  - `nip/cert_format.go` — IdentFrame `cert_format` discriminator
    (`v1` Ed25519 vs. `x509`).
  - `nip/error_codes.go` — NIP error code namespace strings.
  - `nip/verifier.go` — dual-trust IdentFrame verifier (v1 + X.509).
- 21 new tests covering preamble round-trip, X.509 issuance + parsing,
  dual-trust verification, and ACME agent-01 round-trip. Total: 96
  tests green (was 75 at alpha.3).

### Changed

- Version bump to `1.0.0-alpha.4` for suite-wide synchronization with
  the NPS `v1.0.0-alpha.4` release.
- `nip/frames.go` — IdentFrame wire shape extended to carry the
  optional `cert_format` discriminator + leaf `x509_chain` field
  alongside the v1 Ed25519 fields. v1 IdentFrames written by alpha.3
  consumers continue to verify unchanged.

### Suite-wide highlights at alpha.4

- **NPS-RFC-0002 X.509 + ACME** — full cross-SDK port wave (.NET / Java
  / Python / TypeScript / Go / Rust). Servers can now issue dual-trust
  IdentFrames (v1 Ed25519 + X.509 leaf cert chained to a self-signed
  root) and self-onboard NIDs over ACME's `agent-01` challenge type.
- **NPS-CR-0002 — Anchor Node topology queries** — `topology.snapshot`
  / `topology.stream` query types (.NET reference + L2 conformance
  suite). Go consumer-side helpers planned for a later release; the
  Go SDK keeps the alpha.3 `nwp.AnchorNode` shape unchanged.
- **`nps-registry` SQLite-backed real registry** + **`nps-ledger` Phase 2**
  (SQLite + Merkle + STH + inclusion proofs) delivered in the daemon
  repos.

### Covered modules

- core / ncp / nwp / nip (now with `nip/x509`, `nip/acme`) / ndp / nop

---

## [1.0.0-alpha.3] — 2026-04-25

### Changed

- Version bump to `1.0.0-alpha.3` for suite-wide synchronization with the NPS `v1.0.0-alpha.3` release. No functional changes in the Go SDK at this milestone (Go modules version-discover from the git tag, no `version` field to bump).
- 75 tests still green.

### Suite-wide highlights at alpha.3 (per-language helpers planned for alpha.4)

- **NPS-RFC-0001 — NCP connection preamble** (Accepted). Native-mode connections now begin with the literal `b"NPS/1.0\n"` (8 bytes). Reference helper landed in the .NET SDK; Go helper deferred to alpha.4.
- **NPS-RFC-0003 — Agent identity assurance levels** (Accepted). NIP IdentFrame and NWM gain a tri-state `assurance_level` (`anonymous`/`attested`/`verified`). Reference types landed in .NET; Go parity deferred to alpha.4.
- **NPS-RFC-0004 — NID reputation log (CT-style)** (Accepted). Append-only Merkle log entry shape published; reference signer landed in .NET (and shipped as the `nps-ledger` daemon Phase 1). Go helpers deferred to alpha.4.
- **NPS-CR-0001 — Anchor / Bridge node split.** The legacy "Gateway Node" role is renamed to **Anchor Node**; the "translate NPS↔external protocol" role is now its own **Bridge Node** type. AnnounceFrame gained `node_kind` / `cluster_anchor` / `bridge_protocols`. Source-of-truth changes are in `spec/` + the .NET reference implementation.
- **6 NPS resident daemons.** New `daemons/` tree in NPS-Dev defines `npsd` / `nps-runner` / `nps-gateway` / `nps-registry` / `nps-cloud-ca` / `nps-ledger`; `npsd` ships an L1-functional reference and the rest ship as Phase 1 skeletons.

### Covered modules

- core / ncp / nwp / nip / ndp / nop

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

[1.0.0-alpha.4]: https://github.com/labacacia/NPS-sdk-go/releases/tag/v1.0.0-alpha.4
[1.0.0-alpha.3]: https://github.com/LabAcacia/NPS-Dev/releases/tag/v1.0.0-alpha.3
[1.0.0-alpha.2]: https://github.com/LabAcacia/NPS-Dev/releases/tag/v1.0.0-alpha.2
[1.0.0-alpha.1]: https://github.com/LabAcacia/NPS-Dev/releases/tag/v1.0.0-alpha.1
