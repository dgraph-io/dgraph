/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package pb

import "strings"

const tabletKeySep = "@"

// TabletKey returns the composite key for a label tablet. Unlabeled tablets
// use the bare predicate name for backward compatibility.
func TabletKey(predicate, label string) string {
	if label == "" {
		return predicate
	}
	return predicate + tabletKeySep + label
}

// ParseTabletKey splits a composite tablet key into its predicate and label
// components. Uses the rightmost '@' as the separator as a defensive choice,
// though '@' is not valid in Dgraph predicate names (allowed: a-zA-Z0-9_.~).
// For keys without a label (no '@' separator), the label is "".
func ParseTabletKey(key string) (predicate, label string) {
	if idx := strings.LastIndex(key, tabletKeySep); idx >= 0 {
		return key[:idx], key[idx+1:]
	}
	return key, ""
}

// IsLabeled returns true if this tablet has a label assigned via the @label
// schema directive. Labeled tablets are pinned to specific alpha groups and
// receive special routing, rebalancing, and authorization treatment.
func (t *Tablet) IsLabeled() bool {
	return t != nil && t.Label != ""
}

// IsLabeled returns true if this member was started with a --label flag.
// Labeled members serve only predicates whose @label matches their label.
func (m *Member) IsLabeled() bool {
	return m != nil && m.Label != ""
}

// IsLabeled returns true if this schema update carries a @label directive.
// Labeled predicates are routed to the alpha group whose label matches.
func (s *SchemaUpdate) IsLabeled() bool {
	return s != nil && s.Label != ""
}

// TabletIndex provides O(1) nested lookups for tablets by (predicate, label).
// It is a read cache built from the flat proto map[string]*Tablet and avoids
// the O(n) ParseTabletKey scans required by composite-key iteration.
type TabletIndex struct {
	pred map[string]map[string]*Tablet // pred -> label -> *Tablet
}

// NewTabletIndex returns an empty TabletIndex ready for use.
func NewTabletIndex() *TabletIndex {
	return &TabletIndex{pred: make(map[string]map[string]*Tablet)}
}

// Get returns the tablet for the exact (predicate, label) pair, or nil.
func (ti *TabletIndex) Get(predicate, label string) *Tablet {
	labels := ti.pred[predicate]
	if labels == nil {
		return nil
	}
	return labels[label]
}

// Set inserts or updates the tablet for the given (predicate, label) pair.
func (ti *TabletIndex) Set(predicate, label string, tablet *Tablet) {
	labels := ti.pred[predicate]
	if labels == nil {
		labels = make(map[string]*Tablet)
		ti.pred[predicate] = labels
	}
	labels[label] = tablet
}

// Delete removes the tablet for the given (predicate, label) pair.
func (ti *TabletIndex) Delete(predicate, label string) {
	labels := ti.pred[predicate]
	if labels == nil {
		return
	}
	delete(labels, label)
	if len(labels) == 0 {
		delete(ti.pred, predicate)
	}
}

// GetAny returns any tablet for the predicate, preferring the unlabeled one.
// Used by BelongsToReadOnly where we need any group serving this predicate.
func (ti *TabletIndex) GetAny(predicate string) *Tablet {
	labels := ti.pred[predicate]
	if labels == nil {
		return nil
	}
	// Prefer unlabeled tablet.
	if t, ok := labels[""]; ok {
		return t
	}
	// Return any labeled tablet.
	for _, t := range labels {
		return t
	}
	return nil
}

// GetForGroup returns the tablet belonging to gid for the given predicate,
// falling back to any tablet if no own-group match exists. This replaces the
// two-pass aliasing in applyState where own-group tablets won bare-key collisions.
func (ti *TabletIndex) GetForGroup(predicate string, gid uint32) *Tablet {
	labels := ti.pred[predicate]
	if labels == nil {
		return nil
	}
	// First pass: find a tablet belonging to gid.
	for _, t := range labels {
		if t.GroupId == gid {
			return t
		}
	}
	// Fallback: return any tablet for this predicate.
	for _, t := range labels {
		return t
	}
	return nil
}

// AllForPredicate returns the inner label→tablet map for a predicate.
// Returns nil if the predicate is not in the index. O(1).
func (ti *TabletIndex) AllForPredicate(predicate string) map[string]*Tablet {
	return ti.pred[predicate]
}

// HasPredicate returns true if any tablet exists for the given predicate.
func (ti *TabletIndex) HasPredicate(predicate string) bool {
	return len(ti.pred[predicate]) > 0
}

// Len returns the total number of tablets across all predicates.
func (ti *TabletIndex) Len() int {
	n := 0
	for _, labels := range ti.pred {
		n += len(labels)
	}
	return n
}

// Range iterates over all tablets. Return false from fn to stop early.
func (ti *TabletIndex) Range(fn func(pred, label string, tablet *Tablet) bool) {
	for pred, labels := range ti.pred {
		for label, tablet := range labels {
			if !fn(pred, label, tablet) {
				return
			}
		}
	}
}

// BuildFromFlat parses composite keys in a flat proto tablet map and inserts
// them into the nested index. This is the bridge from the proto wire format.
func (ti *TabletIndex) BuildFromFlat(tablets map[string]*Tablet) {
	for key, tablet := range tablets {
		pred, label := ParseTabletKey(key)
		ti.Set(pred, label, tablet)
	}
}
