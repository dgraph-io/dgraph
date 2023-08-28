//go:build integration || upgrade

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package bulk

import (
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/systest/21million/common"
)

func (bsuite *BulkTestSuite) TestQueriesFor21Million() {
	t := bsuite.T()
	require.NoError(t, bsuite.bulkLoader())

	// start alphas
	require.NoError(t, bsuite.StartAlpha())

	// Upgrade
	bsuite.Upgrade()

	require.NoError(t, common.QueriesFor21Million(bsuite.T(), bsuite.dc))
}
