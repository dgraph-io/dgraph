/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package pb

import "testing"

func TestTabletKey_Unlabeled(t *testing.T) {
	got := TabletKey("Document.name", "")
	if got != "Document.name" {
		t.Errorf("TabletKey('Document.name', '') = %q, want 'Document.name'", got)
	}
}

func TestTabletKey_Labeled(t *testing.T) {
	got := TabletKey("Document.name", "secret")
	if got != "Document.name@secret" {
		t.Errorf("TabletKey('Document.name', 'secret') = %q, want 'Document.name@secret'", got)
	}
}

func TestParseTabletKey_Unlabeled(t *testing.T) {
	pred, label := ParseTabletKey("Document.name")
	if pred != "Document.name" || label != "" {
		t.Errorf("ParseTabletKey('Document.name') = (%q, %q), want ('Document.name', '')", pred, label)
	}
}

func TestParseTabletKey_Labeled(t *testing.T) {
	pred, label := ParseTabletKey("Document.name@secret")
	if pred != "Document.name" || label != "secret" {
		t.Errorf("ParseTabletKey('Document.name@secret') = (%q, %q), want ('Document.name', 'secret')", pred, label)
	}
}

func TestParseTabletKey_NamespacedLabeled(t *testing.T) {
	// Dgraph namespaces predicates as "0-Document.name" — the '@' should still
	// be the delimiter even with the namespace prefix.
	pred, label := ParseTabletKey("0-Document.name@top_secret")
	if pred != "0-Document.name" || label != "top_secret" {
		t.Errorf("ParseTabletKey('0-Document.name@top_secret') = (%q, %q), want ('0-Document.name', 'top_secret')", pred, label)
	}
}

func TestTabletKeyRoundTrip(t *testing.T) {
	cases := []struct{ pred, label string }{
		{"Document.name", ""},
		{"Document.name", "secret"},
		{"0-Document.name", "top_secret"},
		{"dgraph.type", ""},
	}
	for _, c := range cases {
		key := TabletKey(c.pred, c.label)
		gotPred, gotLabel := ParseTabletKey(key)
		if gotPred != c.pred || gotLabel != c.label {
			t.Errorf("Round-trip(%q, %q): got (%q, %q)", c.pred, c.label, gotPred, gotLabel)
		}
	}
}
