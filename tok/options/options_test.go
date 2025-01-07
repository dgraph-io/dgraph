/*
 * Copyright 2025 Hypermode Inc. and Contributors
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

package options

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSimpleOptions(t *testing.T) {
	o := NewOptions()
	o.SetOpt("fish", 17)
	floatval, found, err := GetOpt(o, "fish", 32.2)
	require.Equal(t, floatval, 32.2)
	require.True(t, found)
	require.NotNil(t, err)
	intval, found, err := GetOpt(o, "fish", 0)
	require.Equal(t, intval, 17)
	require.True(t, found)
	require.Nil(t, err)
	stringval, found, err := GetOpt(o, "cat", "fishcat")
	require.Equal(t, stringval, "fishcat")
	require.False(t, found)
	require.Nil(t, err)
}

type fooBarType interface {
	DoFoo(fooey int) int
	DoBar(barry int) int
}

type FooBarImpl struct{}

func (fb FooBarImpl) DoFoo(fooey int) int {
	return fooey
}

func (fb FooBarImpl) DoBar(barry int) int {
	return barry + 2
}

func TestInterfaceOptions(t *testing.T) {
	o := NewOptions()
	var fb fooBarType = FooBarImpl{}
	o.SetOpt("foob", fb)
	x, found := GetInterfaceOpt(o, "foob")
	require.Equal(t, x, fb)
	require.True(t, found)
	y, found := GetInterfaceOpt(o, "barb")
	require.Nil(t, y)
	require.False(t, found)
}
