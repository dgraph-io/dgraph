/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package auth

// LevelHierarchy defines the security classification levels in descending order of access.
// Higher index = lower clearance. A user with a higher clearance (lower index) can access
// all levels at or below their clearance.
var LevelHierarchy = []string{"top_secret", "secret", "classified", "unclassified"}

// CanAccess returns true if userLevel can access a predicate with the given label.
// Access is granted if the user's level is at or above the predicate's label in the hierarchy.
func CanAccess(userLevel, predicateLabel string) bool {
	userIdx := indexOf(LevelHierarchy, userLevel)
	predIdx := indexOf(LevelHierarchy, predicateLabel)

	// Unknown levels deny access
	if userIdx == -1 || predIdx == -1 {
		return false
	}

	// Lower index = higher clearance, so userIdx <= predIdx means access granted
	return userIdx <= predIdx
}

// indexOf returns the index of the given level in the hierarchy, or -1 if not found.
func indexOf(hierarchy []string, level string) int {
	for i, l := range hierarchy {
		if l == level {
			return i
		}
	}
	return -1
}
