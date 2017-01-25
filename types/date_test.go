/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package types

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/x"
)

func TestConvertDateToBool(t *testing.T) {
	dt := ValueForType(DateID)
	dt.Value = []byte{}
	_, err := Convert(dt, BoolID)
	if err == nil {
		t.Errorf("Expected error converting date to bool")
	}
}

func TestConvertDateToInt32(t *testing.T) {
	data := []struct {
		in  time.Time
		out int32
	}{
		{createDate(2009, time.November, 10), 1257811200},
		{createDate(1969, time.November, 10), -4492800},
	}
	var dst Val
	var err error
	for _, tc := range data {
		bs := make([]byte, 8)
		binary.LittleEndian.PutUint64(bs, uint64(tc.in.Unix()))
		src := Val{DateID, bs}
		if dst, err = Convert(src, Int32ID); err != nil {
			t.Errorf("Unexpected error converting date to int: %v", err)
		} else if dst.Value.(int32) != tc.out {
			t.Errorf("Converting time to int: Expected %v, got %v", tc.out, dst.Value)
		}
	}
}

func TestConvertDateToFloat(t *testing.T) {
	data := []struct {
		in  time.Time
		out float64
	}{
		{createDate(2009, time.November, 10), 1257811200},
		{createDate(1969, time.November, 10), -4492800},
		{createDate(2039, time.November, 10), 2204496000},
		{createDate(1901, time.November, 10), -2150409600},
	}
	for _, tc := range data {
		bs := make([]byte, 8)
		binary.LittleEndian.PutUint64(bs, uint64(tc.in.Unix()))
		src := Val{DateID, bs}
		if dst, err := Convert(src, FloatID); err != nil {
			t.Errorf("Unexpected error converting date to float: %v", err)
		} else if dst.Value.(float64) != tc.out {
			t.Errorf("Converting date to float: Expected %v, got %v", tc.out, dst.Value)
		}
	}
}

func TestConvertDateToTime(t *testing.T) {
	data := []struct {
		in  time.Time
		out time.Time
	}{
		{createDate(2009, time.November, 10),
			time.Date(2009, time.November, 10, 0, 0, 0, 0, time.UTC)},
		{createDate(1969, time.November, 10),
			time.Date(1969, time.November, 10, 0, 0, 0, 0, time.UTC)},
		{createDate(2039, time.November, 10),
			time.Date(2039, time.November, 10, 0, 0, 0, 0, time.UTC)},
		{createDate(1901, time.November, 10),
			time.Date(1901, time.November, 10, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range data {
		bs := make([]byte, 8)
		binary.LittleEndian.PutUint64(bs, uint64(tc.in.Unix()))
		src := Val{DateID, bs}
		if tout, err := Convert(src, DateTimeID); err != nil {
			t.Errorf("Unexpected error converting date to time: %v", err)
		} else if tout.Value.(time.Time) != tc.out {
			t.Errorf("Converting date to time: Expected %v, got %v", tc.out, tout.Value)
		}
	}
}

func TestConvertInt32ToDate(t *testing.T) {
	data := []struct {
		in  int32
		out time.Time
	}{
		{1257811200, createDate(2009, time.November, 10)},
		{1257894000, createDate(2009, time.November, 10)}, //truncation
		{-4492800, createDate(1969, time.November, 10)},
		{0, createDate(1970, time.January, 1)},
	}
	for _, tc := range data {
		bs := make([]byte, 4)
		binary.LittleEndian.PutUint32(bs[:], uint32(tc.in))
		src := Val{Int32ID, bs[:]}
		if dst, err := Convert(src, DateID); err != nil {
			t.Errorf("Unexpected error converting int to date: %v", err)
		} else if dst.Value.(time.Time) != tc.out {
			t.Errorf("Converting int to date: Expected %v, got %v", tc.out, dst.Value)
		}
	}
}

func TestConvertFloatToDate(t *testing.T) {
	data := []struct {
		in  float64
		out time.Time
	}{
		{1257811200, createDate(2009, time.November, 10)},
		{1257894000.001, createDate(2009, time.November, 10)}, //truncation
		{-4492800, createDate(1969, time.November, 10)},
		{0, createDate(1970, time.January, 1)},
		{2204578800.5, createDate(2039, time.November, 10)},
		{-2150326800.12, createDate(1901, time.November, 10)},
	}
	for _, tc := range data {
		bs := make([]byte, 8)
		u := math.Float64bits(tc.in)
		binary.LittleEndian.PutUint64(bs[:], u)
		src := Val{FloatID, bs[:]}
		if dst, err := Convert(src, DateID); err != nil {
			t.Errorf("Unexpected error converting float to date: %v", err)
		} else if dst.Value.(time.Time) != tc.out {
			t.Errorf("Converting float to date: Expected %v, got %v", tc.out, dst.Value)
		}
	}
}

func TestConvertDateTimeToDate(t *testing.T) {
	data := []struct {
		in  time.Time
		out time.Time
	}{
		{time.Date(2009, time.November, 10, 0, 0, 0, 0, time.UTC),
			createDate(2009, time.November, 10)},
		{time.Date(2009, time.November, 10, 21, 2, 50, 1000000, time.UTC),
			createDate(2009, time.November, 10)}, // truncation
		{time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC),
			createDate(1969, time.November, 10)},
	}
	for _, tc := range data {
		bs, err := tc.in.MarshalBinary()
		require.NoError(t, err)
		src := Val{DateTimeID, bs}
		if dst, err := Convert(src, DateID); err != nil {
			t.Errorf("Unexpected error converting time to date: %v", err)
		} else if dst.Value.(time.Time) != tc.out {
			t.Errorf("Converting time to date: Expected %v, got %v", tc.out, dst.Value)
		}
	}
}

func TestMain(m *testing.M) {
	x.Init()
	os.Exit(m.Run())
}
