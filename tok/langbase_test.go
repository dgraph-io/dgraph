/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package tok

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLangBase(t *testing.T) {
	tests := []struct {
		in, out string
	}{
		{in: "zh", out: "zh"},
		{in: "zh-hant", out: "zh"},
		{in: "zh-hans", out: "zh"},
		{in: "es-es", out: "es"},
		{in: "ar-001", out: "ar"},
		{in: "ca-ES-valencia", out: "ca"},
		{in: "ca-ES-valencia-u-va-posix", out: "ca"},
		{in: "ca-ES-valencia-u-co-phonebk", out: "ca"},
		{in: "ca-ES-valencia-u-co-phonebk-va-posix", out: "ca"},
		{in: "x-klingon", out: "en"},
		{in: "en-US", out: "en"},
		{in: "en-US-u-va-posix", out: "en"},
		{in: "en", out: "en"},
		{in: "en-u-co-phonebk", out: "en"},
		{in: "en-001", out: "en"},
		{in: "sh", out: "sr"},
		{in: "en-GB-u-rg-uszzzz", out: "en"},
		{in: "en-GB-u-rg-uszzzz-va-posix", out: "en"},
		{in: "en-GB-u-co-phonebk-rg-uszzzz", out: "en"},
		{in: "en-GB-u-co-phonebk-rg-uszz", out: "en"},
		{in: "", out: "en"},
		{in: "no_such_language", out: "en"},
		{in: "xxx_such_language", out: "en"},
	}

	var out string
	for _, tc := range tests {
		out = langBase(tc.in)
		require.Equal(t, tc.out, out)
	}
}
