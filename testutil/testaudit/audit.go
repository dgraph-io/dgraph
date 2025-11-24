/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package testaudit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hypermodeinc/dgraph/v25/testutil"
	"github.com/stretchr/testify/require"
)

func VerifyLogs(t *testing.T, path string, cmds []string) {

	abs, err := filepath.Abs(path)
	require.NoError(t, err)

	type log struct {
		Msg string `json:"endpoint"`
	}

	maxRetries := 10
	for attempt := 1; attempt <= maxRetries; attempt++ {
		f, err := os.Open(abs)
		if err != nil {
			if attempt == maxRetries {
				require.NoError(t, err, "failed to open audit log after %d attempts", maxRetries)
			}
			time.Sleep(time.Second)
			continue
		}

		logMap := make(map[string]bool)
		fileScanner := bufio.NewScanner(f)
		for fileScanner.Scan() {
			bytes := fileScanner.Bytes()
			l := new(log)
			if err := json.Unmarshal(bytes, l); err != nil {
				f.Close()
				if attempt == maxRetries {
					require.NoError(t, err, "failed to unmarshal audit log after %d attempts", maxRetries)
				}
				time.Sleep(time.Second)
				continue
			}
			logMap[l.Msg] = true
		}
		f.Close()

		missingCmds := []string{}
		for _, m := range cmds {
			if !logMap[m] {
				missingCmds = append(missingCmds, m)
			}
		}
		if len(missingCmds) == 0 {
			return
		}
		if attempt == maxRetries {
			t.Fatalf("audit logs not present after %d attempts. Missing commands: %v", maxRetries, missingCmds)
		}
		time.Sleep(time.Second)
	}
}

// run manually to generate the encrypted audit log used in TestDecrypt
func TestGenerateAuditForTestDecrypt(t *testing.T) {
	// to generate audit logs, uncomment and run ./t --test=TestGenerateAuditForTestDecrypt
	t.Skip()
	zeroCmd := map[string][]string{
		"/removeNode": {`--location`, "--request", "GET", "--ipv4",
			fmt.Sprintf("%s/removeNode?id=3&group=1", testutil.GetSockAddrZeroHttp())},
		"/assign": {"--location", "--request", "GET", "--ipv4",
			fmt.Sprintf("%s/assign?what=uids&num=100", testutil.GetSockAddrZeroHttp())},
		"/moveTablet": {"--location", "--request", "GET", "--ipv4",
			fmt.Sprintf("%s/moveTablet?tablet=name&group=2", testutil.GetSockAddrZeroHttp())}}

	for _, c := range zeroCmd {
		cmd := exec.Command("curl", c...)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Println(string(out))
			t.Fatal(err)
		}
	}
}
