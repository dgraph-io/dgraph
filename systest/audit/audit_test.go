/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
)

func TestZeroAudit(t *testing.T) {
	state, err := testutil.GetState()
	require.NoError(t, err)
	nId := state.Zeros["1"].Id
	defer os.RemoveAll(fmt.Sprintf("audit_dir/za/zero_audit_0_%s.log", nId))
	zeroCmd := map[string][]string{
		"/removeNode": []string{`--location`, "--request", "GET",
			fmt.Sprintf("%s/removeNode?id=3&group=1", testutil.SockAddrZeroHttp)},
		"/assign": []string{"--location", "--request", "GET",
			fmt.Sprintf("%s/assign?what=uids&num=100", testutil.SockAddrZeroHttp)},
		"/moveTablet": []string{"--location", "--request", "GET",
			fmt.Sprintf("%s/moveTablet?tablet=name&group=2", testutil.SockAddrZeroHttp)}}

	msgs := make([]string, 0)
	// logger is buffered. make calls in bunch so that dont want to wait for flush
	for i := 0; i < 500; i++ {
		for req, c := range zeroCmd {
			msgs = append(msgs, req)
			cmd := exec.Command("curl", c...)
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Println(string(out))
				t.Fatal(err)
			}
		}
	}

	verifyLogs(t, fmt.Sprintf("audit_dir/za/zero_audit_0_%s.log", nId), msgs)
}
func TestAlphaAudit(t *testing.T) {
	state, err := testutil.GetState()
	require.NoError(t, err)
	var nId string
	for key := range state.Groups["1"].Members {
		nId = key
	}
	defer os.Remove(fmt.Sprintf("audit_dir/aa/alpha_audit_1_%s.log", nId))
	testCommand := map[string][]string{
		"/admin": []string{"--location", "--request", "POST",
			fmt.Sprintf("%s/admin", testutil.SockAddrHttp),
			"--header", "Content-Type: application/json",
			"--data-raw", `'{"query":"mutation {\n  backup(
input: {destination: \"/Users/sankalanparajuli/work/backup\"}) {\n    response {\n      message\n      code\n    }\n  }\n}\n","variables":{}}'`},

		"/graphql": []string{"--location", "--request", "POST", fmt.Sprintf("%s/graphql", testutil.SockAddrHttp),
			"--header", "Content-Type: application/json",
			"--data-raw", `'{"query":"query {\n  __schema {\n    __typename\n  }\n}","variables":{}}'`},

		"/alter": []string{"-X", "POST", fmt.Sprintf("%s/alter", testutil.SockAddrHttp), "-d",
			`name: string @index(term) .
			type Person {
			  name
			}`},
		"/query": []string{"-H", "'Content-Type: application/dql'", "-X", "POST", fmt.Sprintf("%s/query", testutil.SockAddrHttp),
			"-d", `$'
			{
			 balances(func: anyofterms(name, "Alice Bob")) {
			   uid
			   name
			   balance
			 }
			}'`},
		"/mutate": []string{"-H", "'Content-Type: application/rdf'", "-X",
			"POST", fmt.Sprintf("%s/mutate?startTs=4", testutil.SockAddrHttp), "-d", `$'
			{
			 set {
			   <0x1> <balance> "110" .
			   <0x1> <dgraph.type> "Balance" .
			   <0x2> <balance> "60" .
			   <0x2> <dgraph.type> "Balance" .
			 }
			}
			'`},
	}

	msgs := make([]string, 0)
	// logger is buffered. make calls in bunch so that dont want to wait for flush
	for i := 0; i < 200; i++ {
		for req, c := range testCommand {
			msgs = append(msgs, req)
			cmd := exec.Command("curl", c...)
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Println(string(out))
				t.Fatal(err)
			}
		}
	}
	verifyLogs(t, fmt.Sprintf("audit_dir/aa/alpha_audit_1_%s.log", nId), msgs)
}

func verifyLogs(t *testing.T, path string, cmds []string) {
	abs, err := filepath.Abs(path)
	require.Nil(t, err)
	f, err := os.Open(abs)
	require.Nil(t, err)

	type log struct {
		Msg string `json:"endpoint"`
	}
	logMap := make(map[string]bool)

	var fileScanner *bufio.Scanner
	fileScanner = bufio.NewScanner(f)
	for fileScanner.Scan() {
		bytes := fileScanner.Bytes()
		l := new(log)
		_ = json.Unmarshal(bytes, l)
		logMap[l.Msg] = true
	}
	for _, m := range cmds {
		if !logMap[m] {
			t.Fatalf("audit logs not present for command %s", m)
		}
	}
}
