// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"net/url"
	"strings"
)

// bridgeParseHTTPEndpoint parses and validates an HTTP(S) Bridge endpoint. By
// default both http:// and https:// are accepted, while private and loopback
// hosts are rejected as an SSRF guard. Faithful port of BridgeEndpointValidator.
func bridgeParseHTTPEndpoint(target *BridgeTarget) (*url.URL, error) {
	if target == nil {
		return nil, newBridgeDispatchError(BridgeErrEndpointInvalid, "bridge_target is required.")
	}

	uri, err := url.Parse(target.Endpoint)
	if err != nil || !uri.IsAbs() || uri.Host == "" ||
		(uri.Scheme != "http" && uri.Scheme != "https") {
		return nil, newBridgeDispatchError(
			BridgeErrEndpointInvalid,
			"bridge_target.endpoint must be an absolute http:// or https:// URI.")
	}

	allowHTTP := bridgeTargetBool(target, "allow_http", true)
	if !allowHTTP && uri.Scheme == "http" {
		return nil, newBridgeDispatchError(
			BridgeErrEndpointInvalid,
			"bridge_target.endpoint MUST use https:// unless bridge_target.allow_http is true.")
	}

	allowedPrefixes := bridgeTargetStringList(target, "allowed_prefixes")
	if len(allowedPrefixes) > 0 {
		matched := false
		for _, prefix := range allowedPrefixes {
			if bridgeMatchesAllowedPrefix(uri, prefix) {
				matched = true
				break
			}
		}
		if !matched {
			return nil, newBridgeDispatchError(
				BridgeErrEndpointInvalid,
				"bridge_target.endpoint '"+target.Endpoint+"' is not in bridge_target.allowed_prefixes.")
		}
	}

	rejectPrivate := bridgeTargetBool(target, "reject_private", true)
	if rejectPrivate && isPrivateHost(uri.Hostname()) {
		return nil, newBridgeDispatchError(
			BridgeErrEndpointInvalid,
			"bridge_target.endpoint host '"+uri.Hostname()+"' is private or loopback (SSRF guard).")
	}

	return uri, nil
}

func bridgeMatchesAllowedPrefix(endpoint *url.URL, rawPrefix string) bool {
	prefix, err := url.Parse(rawPrefix)
	if err != nil || !prefix.IsAbs() {
		return false
	}

	if !strings.EqualFold(endpoint.Scheme, prefix.Scheme) ||
		!strings.EqualFold(endpoint.Hostname(), prefix.Hostname()) ||
		endpoint.Port() != prefix.Port() {
		return false
	}

	prefixPath := prefix.Path
	if prefixPath == "" {
		prefixPath = "/"
	}
	if prefixPath == "/" {
		return true
	}

	endpointPath := endpoint.Path
	if endpointPath == "" {
		endpointPath = "/"
	}
	if !strings.HasPrefix(strings.ToLower(endpointPath), strings.ToLower(prefixPath)) {
		return false
	}

	return len(endpointPath) == len(prefixPath) ||
		strings.HasSuffix(prefixPath, "/") ||
		endpointPath[len(prefixPath)] == '/'
}
