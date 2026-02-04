/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package pb

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
