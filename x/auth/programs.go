/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package auth

// HasProgram returns true if the user has at least one program that matches
// the required programs on the predicate (OR logic).
// If no programs are required (nil or empty), access is granted.
// If programs are required but the user has none, access is denied.
func HasProgram(userPrograms, requiredPrograms []string) bool {
	// No programs required = accessible to all
	if len(requiredPrograms) == 0 {
		return true
	}

	// Programs required but user has none = denied
	if len(userPrograms) == 0 {
		return false
	}

	// OR logic: user needs at least one matching program
	for _, up := range userPrograms {
		for _, rp := range requiredPrograms {
			if up == rp {
				return true
			}
		}
	}

	return false
}
