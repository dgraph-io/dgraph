/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
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
