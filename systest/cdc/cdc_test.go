/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

package cdc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
)

func TestCDC(t *testing.T) {
	defer os.RemoveAll("./cdc_logs/sink.log")
	cmd := exec.Command("dgraph", "increment", "--num", "10",
		"--alpha", testutil.SockAddr)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Println(string(out))
		t.Fatal(err)
	}
	time.Sleep(time.Second * 15)
	verifyCDC(t, "./cdc_logs/sink.log")
}

type CDCEvent struct {
	Value struct {
		Event struct {
			Value int `json:"value"`
		} `json:"event"`
	} `json:"value"`
}

func verifyCDC(t *testing.T, path string) {
	abs, err := filepath.Abs(path)
	require.Nil(t, err)
	f, err := os.Open(abs)
	require.Nil(t, err)
	var fileScanner *bufio.Scanner
	fileScanner = bufio.NewScanner(f)
	iter := 1
	for fileScanner.Scan() {
		bytes := fileScanner.Bytes()
		l := new(CDCEvent)
		err := json.Unmarshal(bytes, l)
		require.Nil(t, err)
		require.Equal(t, iter, l.Value.Event.Value)
		iter = iter + 1
	}
	require.Equal(t, iter, 11)
}
