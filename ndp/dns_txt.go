// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ndp

import (
	"context"
	"net"
	"strconv"
	"strings"
)

const DnsTxtDefaultTTL = 300
const dnsTxtLabelPrefix = "_nps-node."

// DnsTxtLookup is an injectable DNS TXT resolver (for testing).
type DnsTxtLookup interface {
	LookupTXT(ctx context.Context, host string) ([]string, error)
}

// SystemDnsTxtLookup uses the system resolver (net.DefaultResolver).
type SystemDnsTxtLookup struct{}

func (s *SystemDnsTxtLookup) LookupTXT(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupTXT(ctx, host)
}

// ParseNpsTxtRecord parses a single TXT record string into a ResolveResult.
// Returns nil if the record is not a valid NPS TXT record.
func ParseNpsTxtRecord(txt string, host string) *ResolveResult {
	fields := strings.Fields(txt)
	kv := make(map[string]string, len(fields))
	for _, f := range fields {
		idx := strings.IndexByte(f, '=')
		if idx < 0 {
			continue
		}
		kv[f[:idx]] = f[idx+1:]
	}

	// v=nps1 is required
	if kv["v"] != "nps1" {
		return nil
	}

	// nid is required
	nid, ok := kv["nid"]
	if !ok || nid == "" {
		return nil
	}

	// port defaults to 17433
	port := uint64(17433)
	if ps, ok := kv["port"]; ok && ps != "" {
		p, err := strconv.ParseUint(ps, 10, 64)
		if err == nil {
			port = p
		}
	}

	return &ResolveResult{
		Host:            host,
		Port:            port,
		Protocol:        "https",
		CertFingerprint: kv["fp"],
	}
}

// ExtractHostFromTarget extracts the hostname from a nwp:// URL.
func ExtractHostFromTarget(target string) string {
	const prefix = "nwp://"
	if !strings.HasPrefix(target, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(target, prefix)
	idx := strings.Index(rest, "/")
	if idx < 0 {
		return rest
	}
	return rest[:idx]
}
