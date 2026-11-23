/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestGetEncAclKeysEncKeyLength(t *testing.T) {
	tests := []struct {
		length int
		valid  bool
	}{
		{0, false},
		{15, false},
		{16, true},
		{24, true},
		{32, true},
		{33, false},
		{64, false},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d bytes", tc.length), func(t *testing.T) {
			keyFile := filepath.Join(t.TempDir(), "enc_key")
			require.NoError(t, os.WriteFile(keyFile, make([]byte, tc.length), 0600))

			config := viper.New()
			config.Set("encryption", BuildEncFlag(keyFile))

			keys, err := GetEncAclKeys(config)
			if !tc.valid {
				require.ErrorContains(t, err, "encryption key must have length of 16, 24, or 32 bytes")
				return
			}
			require.NoError(t, err)
			require.Len(t, keys.EncKey, tc.length)
		})
	}
}

func TestGetEncAclKeysNoEncKey(t *testing.T) {
	keys, err := GetEncAclKeys(viper.New())
	require.NoError(t, err)
	require.Nil(t, keys.EncKey)
}
