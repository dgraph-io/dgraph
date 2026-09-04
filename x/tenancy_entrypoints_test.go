/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// clearNamespaceSites is every function that strips the client-supplied namespace,
// and why that function is the right place for it.
//
// This is a decision table, not a description. ClearIncomingNamespace belongs at an
// entry point and nowhere else: md["namespace"] is client-controlled server-side and
// the built-in resolver leaves it in place when it cannot derive one from a
// credential, so an entry point that skips the call attributes the request to
// whatever tenant the caller named. A shared continuation that makes the call is the
// mirror-image bug — it discards the namespace an already-attributed in-process
// caller was given.
//
// The behavioral tests in edgraph and worker prove each of these four rejects an
// uncredentialed request. This table adds what behavior cannot see: that the call
// sits in *this* function rather than having drifted down into the continuation
// below it, where it would still look correct from the outside.
var clearNamespaceSites = map[string]string{
	"edgraph/query.go:RunDQL": "gRPC entry point; resolves its own tenant",
	"edgraph/server.go:Alter": "entry point, not alter(), which also serves AlterNoAuth " +
		"with a context an in-process caller has already attributed",
	"edgraph/server.go:Query": "entry point, not QueryNoGrpc(), which also serves the two " +
		"HTTP handlers and GetGQLSchema's trusted tenancy",
	"worker/zero_proxy.go:forwardAssignUidsToZero": "gRPC entry point for UID leasing, and " +
		"the only gate on it — without this an uncredentialed caller leases UIDs in any tenant",
}

// clearNamespaceScanRoot is the repo root, walked in full: a scan that can miss a
// call site cannot report completeness. See capabilityScanRoot in edgraph.
const clearNamespaceScanRoot = ".."

var clearNamespaceScanSkip = map[string]bool{
	".git": true, "vendor": true, "testdata": true, "protos": true,
	"compose": true, "contrib": true, ".trunk": true, "systest": true,
	// A nested checkout under the repo root would be walked as if it were part of
	// this one, so the same function is found twice and the count assertion fails.
	// Agent worktrees land here by convention.
	".worktrees": true,
}

// scanClearNamespaceSites returns "dir/file.go:FuncName" for every function whose
// body calls ClearIncomingNamespace.
func scanClearNamespaceSites(t *testing.T) map[string]bool {
	t.Helper()
	found := make(map[string]bool)
	var parsed int

	err := filepath.WalkDir(clearNamespaceScanRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if clearNamespaceScanSkip[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Contains(src, []byte("ClearIncomingNamespace")) {
			return nil
		}
		parsed++

		fset := token.NewFileSet()
		af, parseErr := parser.ParseFile(fset, path, src, 0)
		require.NoError(t, parseErr, "parsing %s", path)

		for _, decl := range af.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			// The declaration itself is not a call site.
			if fd.Name.Name == "ClearIncomingNamespace" {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || clearNamespaceCallee(call.Fun) != "ClearIncomingNamespace" {
					return true
				}
				found[clearNamespaceSitePath(path)+":"+fd.Name.Name] = true
				return true
			})
		}
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, parsed, "the walk parsed no files mentioning ClearIncomingNamespace; "+
		"clearNamespaceScanRoot is wrong and this test proves nothing")
	return found
}

func clearNamespaceCallee(fn ast.Expr) string {
	switch f := fn.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

func clearNamespaceSitePath(path string) string {
	clean := filepath.ToSlash(filepath.Clean(path))
	return strings.TrimPrefix(clean, "../")
}

// TestEveryEntryPointClearsIncomingNamespace pins the guard's call sites in both
// directions. A deleted call is a privilege escalation; a call added to a shared
// continuation silently strips tenancy from trusted in-process callers.
func TestEveryEntryPointClearsIncomingNamespace(t *testing.T) {
	found := scanClearNamespaceSites(t)

	for site := range found {
		if _, declared := clearNamespaceSites[site]; !declared {
			t.Errorf("%s clears the incoming namespace but is not in clearNamespaceSites. "+
				"If it is an entry point, add it and say so. If it is a continuation of an "+
				"already-attributed request, the call is wrong: it discards the namespace an "+
				"in-process caller supplied.", site)
		}
	}

	for site, why := range clearNamespaceSites {
		if !found[site] {
			t.Errorf("clearNamespaceSites declares %s (%s), which no longer clears the "+
				"incoming namespace. Deleting that call lets an uncredentialed caller act in "+
				"whichever tenant they name in md[\"namespace\"].", site, why)
		}
	}
}

// TestClearNamespaceSiteCoverage guards against the table or the scan going empty,
// which would make the test above vacuously pass.
func TestClearNamespaceSiteCoverage(t *testing.T) {
	found := scanClearNamespaceSites(t)
	require.NotEmpty(t, found, "the scan found no entry-point guards at all")
	require.Len(t, found, len(clearNamespaceSites))
}
