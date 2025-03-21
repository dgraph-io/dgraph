/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package dql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseDQL(t *testing.T) {
	testCases := []string{
		`{ set {_:a <name> "Alice" .}}`,
		"schema{}",
		`upsert {
			query {
				ns(func: eq(dgraph.namespace.name, "ns1")) {
					q as var
					dgraph.namespace.id
				}
			}

			mutation {
				delete {
					uid(q) * * .
				}
			}
		}`,
		`{
			q(func: eq(dgraph.namespace.name, "ns1")) {
				uid
				dgraph.type
				dgraph.namespace.id
			}
		}`,
		` query test($a: int) {
			q(func: uid(0x1)) {
				x as count(uid)
				p : math(x + $a)
			}
		}`,
		`{
			me(func: uid(1)) {
				Upvote {
					u as Author
				}
				count(val(u))
			}
		}`,
		`{
			user(func: uid(0x0a)) {
				...fragmenta,...fragmentb
				friends {
					name
				}
				...fragmentc
				hobbies
				...fragmentd
			}
			me(func: uid(0x01)) {
				...fragmenta
				...fragmentb
			}
		}
		fragment fragmenta {
			name
		}
		fragment fragmentb {
			id
		}
		fragment fragmentc {
			name
		}
		fragment fragmentd {
			id
		}`,
		`{
			set {
				<name> <is> <something> .
				<hometown> <is> <san/francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		}`,
	}

	for _, tc := range testCases {
		_, err := ParseDQL(tc)
		require.NoError(t, err)
	}
}
