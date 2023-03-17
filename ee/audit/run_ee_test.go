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
	"crypto/aes"
	"os"
	"testing"
)

func check(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

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

	filePath := "testfiles/zero_audit_0_1.log.enc"
	copyPath := "testfiles/zero_audit_0_1.log.enc.copy"

	// during test we will truncate file
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

	file.Truncate(2000) //joshua: write out tests
	decrypt(file, outfile, block, stat.Size())
}
