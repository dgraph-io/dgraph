/*
 * Copyright (C) 2019 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseTime(t *testing.T) {
	tests := []struct {
		in, failure string
		out         time.Time
	}{
		{in: "2018-10-28T04:00:10",
			out: time.Date(2018, 10, 28, 4, 00, 10, 0, time.UTC)},
		{in: "2018-10-28T04:00:10Z",
			out: time.Date(2018, 10, 28, 4, 00, 10, 0, time.UTC)},
		{in: "2018-10-28T04:00:10-00:00",
			out: time.Date(2018, 10, 28, 4, 00, 10, 0, time.FixedZone("", 0))},
		{in: "2018-05-30T09:30:10.5",
			out: time.Date(2018, 5, 30, 9, 30, 10, 500000000, time.UTC)},
		{in: "2018-05-30T09:30:10.5Z",
			out: time.Date(2018, 5, 30, 9, 30, 10, 500000000, time.UTC)},
		{in: "2018-05-30T09:30:10.5-00:00",
			out: time.Date(2018, 5, 30, 9, 30, 10, 500000000, time.FixedZone("", 0))},
		{in: "2018-05-30T09:30:10-06:00",
			out: time.Date(2018, 5, 30, 9, 30, 10, 0, time.FixedZone("", -6*60*60))},
		{in: "2018-05-28T14:41:57+30:00",
			out: time.Date(2018, 5, 28, 14, 41, 57, 0, time.FixedZone("", 30*60*60))},
	}
	for _, tc := range tests {
		out, err := ParseTime(tc.in)
		if tc.failure != "" {
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.failure)
		} else {
			require.NoError(t, err)
			require.EqualValues(t, tc.out, out)
		}
	}
}
