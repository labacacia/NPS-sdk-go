// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

// Package x509 — NPS X.509 NID certificate primitives per NPS-RFC-0002 §4.
package x509

import "encoding/asn1"

// The 1.3.6.1.4.1.99999 arc is provisional pending IANA Private Enterprise
// Number assignment (NPS-RFC-0002 §10 OQ-2). Implementations MUST update
// these constants once the official PEN is granted.
var (
	// EKU OIDs (NPS-RFC-0002 §4.1).
	OidEkuAgentIdentity        = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1, 1}
	OidEkuNodeIdentity         = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1, 2}
	OidEkuCaIntermediateAgent  = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1, 3}

	// Custom extensions.
	OidNidAssuranceLevel = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 2, 1}
)
