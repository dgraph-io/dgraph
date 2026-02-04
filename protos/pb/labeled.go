/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package pb

import "strings"

const tabletKeySep = "@"

// TabletKey returns the composite key for a sub-tablet. Unlabeled sub-tablets
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
