/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import "testing"

func TestEntityLabelCache_GetSet(t *testing.T) {
	c := newEntityLabelCache(100)
	c.Set(42, "secret")
	label, ok := c.Get(42)
	if !ok || label != "secret" {
		t.Errorf("Get(42) = (%q, %v), want ('secret', true)", label, ok)
	}
}

func TestEntityLabelCache_Miss(t *testing.T) {
	c := newEntityLabelCache(100)
	label, ok := c.Get(99)
	if ok || label != "" {
		t.Errorf("Get(99) = (%q, %v), want ('', false)", label, ok)
	}
}

func TestEntityLabelCache_Invalidate(t *testing.T) {
	c := newEntityLabelCache(100)
	c.Set(42, "secret")
	c.Invalidate(42)
	label, ok := c.Get(42)
	if ok {
		t.Errorf("Get(42) after Invalidate = (%q, %v), want ('', false)", label, ok)
	}
}

func TestEntityLabelCache_Clear(t *testing.T) {
	c := newEntityLabelCache(100)
	c.Set(1, "a")
	c.Set(2, "b")
	c.Clear()
	if _, ok := c.Get(1); ok {
		t.Error("Get(1) after Clear should miss")
	}
	if _, ok := c.Get(2); ok {
		t.Error("Get(2) after Clear should miss")
	}
}

func TestEntityLabelCache_UnlabeledEntity(t *testing.T) {
	// An entity with no label should be cached as "" (empty string)
	// so we don't repeatedly look it up from group 1.
	c := newEntityLabelCache(100)
	c.Set(42, "")
	label, ok := c.Get(42)
	if !ok || label != "" {
		t.Errorf("Get(42) = (%q, %v), want ('', true)", label, ok)
	}
}
