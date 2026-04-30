// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip

// NIP error code wire constants — mirror of spec/error-codes.md NIP section.
const (
	// Cert verification (v1 + v2).
	ErrCertExpired           = "NIP-CERT-EXPIRED"
	ErrCertRevoked           = "NIP-CERT-REVOKED"
	ErrCertSignatureInvalid  = "NIP-CERT-SIGNATURE-INVALID"
	ErrCertUntrustedIssuer   = "NIP-CERT-UNTRUSTED-ISSUER"
	ErrCertCapabilityMissing = "NIP-CERT-CAPABILITY-MISSING"
	ErrCertScopeViolation    = "NIP-CERT-SCOPE-VIOLATION"

	// CA service.
	ErrCaNidNotFound          = "NIP-CA-NID-NOT-FOUND"
	ErrCaNidAlreadyExists     = "NIP-CA-NID-ALREADY-EXISTS"
	ErrCaSerialDuplicate      = "NIP-CA-SERIAL-DUPLICATE"
	ErrCaRenewalTooEarly      = "NIP-CA-RENEWAL-TOO-EARLY"
	ErrCaScopeExpansionDenied = "NIP-CA-SCOPE-EXPANSION-DENIED"

	ErrOcspUnavailable    = "NIP-OCSP-UNAVAILABLE"
	ErrTrustFrameInvalid  = "NIP-TRUST-FRAME-INVALID"

	// RFC-0003 (assurance level).
	ErrAssuranceMismatch = "NIP-ASSURANCE-MISMATCH"
	ErrAssuranceUnknown  = "NIP-ASSURANCE-UNKNOWN"

	// RFC-0004 (reputation log).
	ErrReputationEntryInvalid   = "NIP-REPUTATION-ENTRY-INVALID"
	ErrReputationLogUnreachable = "NIP-REPUTATION-LOG-UNREACHABLE"

	// RFC-0002 (X.509 + ACME).
	ErrCertFormatInvalid       = "NIP-CERT-FORMAT-INVALID"
	ErrCertEkuMissing          = "NIP-CERT-EKU-MISSING"
	ErrCertSubjectNidMismatch  = "NIP-CERT-SUBJECT-NID-MISMATCH"
	ErrAcmeChallengeFailed     = "NIP-ACME-CHALLENGE-FAILED"
)
