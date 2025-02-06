/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package pb

// Sensitive implements the Stringer interface to redact its contents.
// Use this type for sensitive info such as keys, passwords, or secrets
// so it doesn't leak as output such as logs.
type Sensitive string

func (Sensitive) String() string {
	return "****"
}
