/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package audit

import (
	"bufio"
	"crypto/aes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func check(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

// we will truncate copy of encrypted audit log file
func copy(t *testing.T, src string, dst string) {
	// could also us io.CopyN but this is a small file
	data, err := os.ReadFile(src)
	check(t, err)
	err = os.WriteFile(dst, data, 0666)
	check(t, err)
}

func TestDecrypt(t *testing.T) {
	key, err := os.ReadFile("../enc/test-fixtures/enc-key")
	check(t, err)

	// encrypted audit log generated using dgraph v22.0.2
	filePath := "testfiles/zero_audit_0_1.log.enc"
	copyPath := "testfiles/zero_audit_0_1.log.enc.copy"

	// during test we will truncate copy of file
	copy(t, filePath, copyPath)
	defer os.RemoveAll(copyPath)

	file, err := os.OpenFile(copyPath, os.O_RDWR, 0666)
	check(t, err)
	defer func() {
		if err := file.Close(); err != nil {
			t.Fatal("error closing file")
		}
	}()

	stat, err := os.Stat(copyPath)
	check(t, err)
	sz := stat.Size() // get size of audit log

	outfile, err := os.OpenFile("testfiles/zero_audit_0_1.log",
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	check(t, err)
	defer func() {
		if err := outfile.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	//joshua: defer os.RemoveAll("testfiles/zero_audit_0_1.log")
	block, err := aes.NewCipher(key)
	check(t, err)

	file.Truncate(sz) //joshua: write out tests
	decrypt(file, outfile, block, sz)
	verifyLogs(t, "testfiles/zero_audit_0_1.log", expect())
}

func expect() []string {
	zeroCmd := map[string]string{
		"/removeNode": "",
		"/assign":     "",
		"/moveTablet": ""}

	// generate expected audit log contents
	msgs := make([]string, 0)
	for i := 0; i < 500; i++ {
		for req := range zeroCmd {
			msgs = append(msgs, req)
		}
	}

	return msgs
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

	fileScanner := bufio.NewScanner(f)
	for fileScanner.Scan() {
		bytes := fileScanner.Bytes()
		l := new(log)
		_ = json.Unmarshal(bytes, l)
		logMap[l.Msg] = true
	}
	fmt.Println(logMap)
	for _, m := range cmds {
		if !logMap[m] {
			t.Fatalf("audit logs not present for command %s", m)
		}
	}
}
