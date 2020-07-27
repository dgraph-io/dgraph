/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package query

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRDFResult(t *testing.T) {
	query := `{
		friends_15_and_19(func: uid(1)) {
		  name
		  friend @filter(ge(age, 15) AND lt(age, 19)) {
			  name
			  age
		  }
		}
	  }`

	rdf, err := processQueryRDF(context.Background(), t, query)
	require.NoError(t, err)
	require.Equal(t, string(rdf), `<0x1> <name> "Michonne" .
<0x1> <friend> <0x17> .
<0x1> <friend> <0x18> .
<0x1> <friend> <0x19> .
<0x17> <name> "Rick Grimes" .
<0x18> <name> "Glenn Rhee" .
<0x19> <name> "Daryl Dixon" .
<0x17> <age> 15 .
<0x18> <age> 15 .
<0x19> <age> 17 .
`)
}
