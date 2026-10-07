// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"regexp"
	"strings"
)

// scpLikeSSHAddrPattern matches scp-like ssh remote addresses "[user@]host:path"
// where "host" is a hostname or IP without a port. IPv6 literals and custom
// ports are not expressible in scp-like syntax, use the "ssh://" form instead.
var scpLikeSSHAddrPattern = regexp.MustCompile(`^[a-zA-Z0-9._~-]+@[a-zA-Z0-9._-]+:[^/].*$`)

// NormalizeScpLikeSSHAddr converts a scp-like ssh address such as
// "git@github.com:owner/repo.git" into the canonical "ssh://git@github.com/owner/repo.git"
// form, so that it can be parsed and validated by net/url based code.
// Any other address is returned unchanged.
func NormalizeScpLikeSSHAddr(addr string) string {
	if !scpLikeSSHAddrPattern.MatchString(addr) {
		return addr
	}
	user, rest, _ := strings.Cut(addr, "@")
	host, path, _ := strings.Cut(rest, ":")
	return "ssh://" + user + "@" + host + "/" + strings.TrimPrefix(path, "/")
}
