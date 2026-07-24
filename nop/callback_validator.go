// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateCallbackURL validates a TaskFrame.callback_url per NPS-5 §8.4:
//   - MUST be an https:// URL.
//   - SHOULD NOT target a private/loopback address (SSRF guard).
//
// It returns the empty string when valid, otherwise a human-readable error.
func ValidateCallbackURL(callbackURL string) string {
	if strings.TrimSpace(callbackURL) == "" {
		return "callback_url must not be empty."
	}

	u, err := url.Parse(callbackURL)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return fmt.Sprintf("callback_url '%s' is not a valid absolute URI.", callbackURL)
	}

	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Sprintf("callback_url MUST use the https:// scheme (got '%s://').", u.Scheme)
	}

	host := u.Hostname()
	if IsPrivateHost(host) {
		return fmt.Sprintf("callback_url host '%s' resolves to a private or loopback address (SSRF guard).", host)
	}

	return ""
}

// IsPrivateHost reports whether host is a well-known private / loopback /
// link-local address or hostname, without performing DNS resolution.
func IsPrivateHost(host string) bool {
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}

	// url.Hostname already strips IPv6 brackets; guard anyway.
	stripped := strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if ip := net.ParseIP(stripped); ip != nil {
		return isPrivateIP(ip)
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 127: // 127.0.0.0/8 loopback
			return true
		case v4[0] == 10: // 10.0.0.0/8
			return true
		case v4[0] == 0: // 0.0.0.0/8
			return true
		case v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31: // 172.16.0.0/12
			return true
		case v4[0] == 192 && v4[1] == 168: // 192.168.0.0/16
			return true
		case v4[0] == 169 && v4[1] == 254: // 169.254.0.0/16 link-local
			return true
		}
		return false
	}

	// IPv6
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}
