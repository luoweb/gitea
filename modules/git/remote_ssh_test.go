// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeScpLikeSSHAddr(t *testing.T) {
	cases := []struct {
		addr string
		want string
	}{
		{"git@github.com:owner/repo.git", "ssh://git@github.com/owner/repo.git"},
		{"user-name@example.com:path/to/repo.git", "ssh://user-name@example.com/path/to/repo.git"},
		{"git@192.168.1.10:owner/repo.git", "ssh://git@192.168.1.10/owner/repo.git"},
		{"ssh://git@github.com/owner/repo.git", "ssh://git@github.com/owner/repo.git"},
		{"ssh://git@github.com:2222/owner/repo.git", "ssh://git@github.com:2222/owner/repo.git"},
		{"https://github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"https://user:pass@github.com/owner/repo.git", "https://user:pass@github.com/owner/repo.git"},
		{"git://github.com/owner/repo.git", "git://github.com/owner/repo.git"},
		{"/home/foo/bar/goo", "/home/foo/bar/goo"},
		{"C:\\Users\\repo", "C:\\Users\\repo"},
		{"owner/repo.git", "owner/repo.git"},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.addr, func(t *testing.T) {
			assert.Equal(t, c.want, NormalizeScpLikeSSHAddr(c.addr))
		})
	}
}

func TestParseRemoteAddrSSH(t *testing.T) {
	// scp-like addresses are normalized to the ssh:// form and pass through
	addr, err := ParseRemoteAddr("git@github.com:owner/repo.git", "", "")
	assert.NoError(t, err)
	assert.Equal(t, "ssh://git@github.com/owner/repo.git", addr)

	addr, err = ParseRemoteAddr("ssh://git@github.com:2222/owner/repo.git", "", "")
	assert.NoError(t, err)
	assert.Equal(t, "ssh://git@github.com:2222/owner/repo.git", addr)

	// http(s) credentials embedding keeps working as before
	addr, err = ParseRemoteAddr("https://github.com/owner/repo.git", "user", "pass")
	assert.NoError(t, err)
	assert.Equal(t, "https://user:pass@github.com/owner/repo.git", addr)
}
