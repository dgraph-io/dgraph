/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanRelPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"dot", ".", ""},
		{"dot dot", "..", ""},
		{"only dot dot segments", "../../..", ""},
		{"leading dot segment", "./a/b", filepath.FromSlash("a/b")},
		{"internal dot dot resolved", "a/../b", "b"},
		{"internal dot dot contained", "a/../../b", "b"},
		{"leading dot dot", "../../etc/passwd", filepath.FromSlash("etc/passwd")},
		{"absolute path anchored", "/etc/hosts", filepath.FromSlash("etc/hosts")},
		{"absolute root", "/", ""},
		{"trailing separator", "a/b/", filepath.FromSlash("a/b")},
		{"duplicate separators", "a//b///c", filepath.FromSlash("a/b/c")},
		{"legitimate backup path unchanged",
			"dgraph.20260101.120000.000/r42-g1.backup",
			filepath.FromSlash("dgraph.20260101.120000.000/r42-g1.backup")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, cleanRelPath(tc.in))
		})
	}
}

// escapesRoot reports whether target is outside base.
func escapesRoot(t *testing.T, base, target string) bool {
	t.Helper()
	rel, err := filepath.Rel(base, target)
	require.NoError(t, err)
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func TestFileHandlerJoinPathContainment(t *testing.T) {
	root := t.TempDir()
	h := &fileHandler{rootDir: root + string(filepath.Separator), prefix: "backups"}
	base := filepath.Join(root, "backups")

	traversals := []string{
		"../../../../etc/passwd",
		"dgraph.20260101.120000.000/../../../../../../etc/shadow",
		filepath.Join("..", "..", "secret.txt"),
		"/etc/hosts",
	}
	for _, p := range traversals {
		got := h.JoinPath(p)
		require.Falsef(t, escapesRoot(t, base, got),
			"JoinPath(%q) escaped handler root: %q", p, got)
	}

	// A legitimate relative backup path must be untouched.
	good := filepath.Join("dgraph.20260101.120000.000", backupName(42, 1))
	require.Equal(t, filepath.Join(base, good), h.JoinPath(good))
}

func TestS3HandlerGetObjectPathContainment(t *testing.T) {
	h := &s3Handler{objectPrefix: "dgraph/backups"}

	traversals := []string{
		"../../../../etc/passwd",
		"dgraph.1/../../../../other/secret",
		"/etc/hosts",
	}
	for _, p := range traversals {
		got := h.getObjectPath(p)
		require.Falsef(t, strings.HasPrefix(got, ".."),
			"getObjectPath(%q) escaped object prefix: %q", p, got)
	}

	// A legitimate relative object path must be untouched.
	good := filepath.Join("dgraph.1", backupName(42, 1))
	require.Equal(t, filepath.Join("dgraph/backups", good), h.getObjectPath(good))
}
