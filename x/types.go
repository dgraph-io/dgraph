/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

type ExportedGQLSchema struct {
	Namespace uint64
	Schema    string
}

// Sensitive implements the Stringer interface to redact its contents.
// Use this type for sensitive info such as keys, passwords, or secrets so it doesn't leak
// as output such as logs.
type Sensitive []byte

func (Sensitive) String() string {
	return "****"
}
