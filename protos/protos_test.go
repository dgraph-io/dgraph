//go:build integration

/* Copyright 2025 Hypermode Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package protos

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtosRegenerate(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping test on non-Linux platform")
	}

	// Run make regenerate
	cmd := exec.Command("make", "regenerate")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Got error while regenerating protos: %s", output)

	// Check if generated files changed
	generatedProtos := filepath.Join("pb", "pb.pb.go")
	diffCmd := exec.Command("git", "diff", "--quiet", "--", generatedProtos)
	fmt.Printf("diffCmd: %+v\n", diffCmd)
	err = diffCmd.Run()
	require.NoError(t, err, "pb.pb.go changed after regenerating")
}
