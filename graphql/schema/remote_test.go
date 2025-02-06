/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGqlType_String(t *testing.T) {
	tcases := []struct {
		name            string
		gqlType         *gqlType
		expectedTypeStr string
	}{
		{
			name:            "Nil type gives empty string",
			gqlType:         nil,
			expectedTypeStr: "",
		},
		{
			name: "Scalar type",
			gqlType: &gqlType{
				Kind:   "SCALAR",
				Name:   "Int",
				OfType: nil,
			},
			expectedTypeStr: "Int",
		},
		{
			name: "Non-null Scalar type",
			gqlType: &gqlType{
				Kind: "NON_NULL",
				Name: "",
				OfType: &gqlType{
					Kind:   "SCALAR",
					Name:   "String",
					OfType: nil,
				},
			},
			expectedTypeStr: "String!",
		},
		{
			name: "Object type",
			gqlType: &gqlType{
				Kind:   "OBJECT",
				Name:   "Author",
				OfType: nil,
			},
			expectedTypeStr: "Author",
		},
		{
			name: "Non-null Object type",
			gqlType: &gqlType{
				Kind: "NON_NULL",
				Name: "",
				OfType: &gqlType{
					Kind:   "OBJECT",
					Name:   "Author",
					OfType: nil,
				},
			},
			expectedTypeStr: "Author!",
		},
		{
			name: "List of Scalar type",
			gqlType: &gqlType{
				Kind: "LIST",
				Name: "",
				OfType: &gqlType{
					Kind:   "SCALAR",
					Name:   "ID",
					OfType: nil,
				},
			},
			expectedTypeStr: "[ID]", // TODO: interpret ID as String
		},
		{
			name: "List of Non-null Scalar type",
			gqlType: &gqlType{
				Kind: "LIST",
				Name: "",
				OfType: &gqlType{
					Kind: "NON_NULL",
					Name: "",
					OfType: &gqlType{
						Kind:   "SCALAR",
						Name:   "Float",
						OfType: nil,
					},
				},
			},
			expectedTypeStr: "[Float!]",
		},
		{
			name: "Non-null List of Non-null Scalar type",
			gqlType: &gqlType{
				Kind: "NON_NULL",
				Name: "",
				OfType: &gqlType{
					Kind: "LIST",
					Name: "",
					OfType: &gqlType{
						Kind: "NON_NULL",
						Name: "",
						OfType: &gqlType{
							Kind:   "SCALAR",
							Name:   "Boolean",
							OfType: nil,
						},
					},
				},
			},
			expectedTypeStr: "[Boolean!]!",
		},
		{
			name: "List of Object type",
			gqlType: &gqlType{
				Kind: "LIST",
				Name: "",
				OfType: &gqlType{
					Kind:   "OBJECT",
					Name:   "Author",
					OfType: nil,
				},
			},
			expectedTypeStr: "[Author]",
		},
		{
			name: "List of Non-null Object type",
			gqlType: &gqlType{
				Kind: "LIST",
				Name: "",
				OfType: &gqlType{
					Kind: "NON_NULL",
					Name: "",
					OfType: &gqlType{
						Kind:   "OBJECT",
						Name:   "Author",
						OfType: nil,
					},
				},
			},
			expectedTypeStr: "[Author!]",
		},
		{
			name: "Non-null List of Non-null Object type",
			gqlType: &gqlType{
				Kind: "NON_NULL",
				Name: "",
				OfType: &gqlType{
					Kind: "LIST",
					Name: "",
					OfType: &gqlType{
						Kind: "NON_NULL",
						Name: "",
						OfType: &gqlType{
							Kind:   "OBJECT",
							Name:   "Author",
							OfType: nil,
						},
					},
				},
			},
			expectedTypeStr: "[Author!]!",
		},
		{
			name: "Non-null List of List of List of Non-Null Object type",
			gqlType: &gqlType{
				Kind: "NON_NULL",
				Name: "",
				OfType: &gqlType{
					Kind: "LIST",
					Name: "",
					OfType: &gqlType{
						Kind: "LIST",
						Name: "",
						OfType: &gqlType{
							Kind: "LIST",
							Name: "",
							OfType: &gqlType{
								Kind: "NON_NULL",
								Name: "",
								OfType: &gqlType{
									Kind:   "OBJECT",
									Name:   "Author",
									OfType: nil,
								},
							},
						},
					},
				},
			},
			expectedTypeStr: "[[[Author!]]]!",
		},
	}

	for _, tcase := range tcases {
		t.Run(tcase.name, func(t *testing.T) {
			require.Equal(t, tcase.expectedTypeStr, tcase.gqlType.String())
		})
	}
}
