/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"flag"
	"fmt"
	"testing"
)

func TestMain(m *testing.M) {
	flag.Parse()
	if err := flag.Set("debugmode", "true"); err != nil {
		fmt.Printf("error setting debug mode: %v\n", err)
	}

	m.Run()
}
