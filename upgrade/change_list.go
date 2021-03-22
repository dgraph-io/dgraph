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

package upgrade

func init() {
	allChanges = changeList{
		{
			introducedIn: &version{major: 20, minor: 3, patch: 0},
			changes: []*change{{
				name: "Upgrade ACL Rules",
				description: "This updates the ACL nodes to use the new ACL model introduced in" +
					" v20.03.0. This is applied while upgrading from v1.2.2+ to v20.03.0+",
				minFromVersion: &version{major: 1, minor: 2, patch: 2},
				applyFunc:      upgradeACLRules,
			}},
		},
		{
			introducedIn: &version{major: 20, minor: 7, patch: 0},
			changes: []*change{
				{
					name: "Upgrade ACL Type names",
					description: "This updates the ACL nodes to use the new type names for the" +
						" types User, Group and Rule. They are now dgraph.type.User, " +
						"dgraph.type.Group, and dgraph.type.Rule. This change was introduced in " +
						"v20.07.0, and is applied while upgrading from v20.03.0+ to v20.07.0+. " +
						"For more info, see: https://github.com/dgraph-io/dgraph/pull/5185",
					minFromVersion: &version{major: 20, minor: 3, patch: 0},
					applyFunc:      upgradeAclTypeNames,
				},
				// another change in 20.07.0 version is if someone was having types/predicates in
				// reserved namespace, then data should be migrated for such cases,
				// to not use the reserved namespace. Migration is expected to be done by the user,
				// and we will only provide a documentation mentioning the steps to be taken.
				// This is not being automated because:
				// 	1. There is very less chance that someone was using the reserved namespace,
				//	so this automation may not be used at all.
				// 	2. The data migration can be huge, if needed to migrate. The automated code
				//	can't handle every scenario. So, it is best to let the user do it.
			},
		},
		{
			introducedIn: &version{major: 21, minor: 3, patch: 0},
			changes: []*change{
				{
					name: "Upgrade Persistent Query",
					description: "This updates the persisted query from old format to new format." +
						"Persistent query had 2 predicates which have been merged into a single " +
						"predicate dgraph.graphql.p_query. " +
						"For more info, see: https://github.com/dgraph-io/dgraph/pull/7451",
					minFromVersion: &version{major: 20, minor: 11, patch: 0},
					applyFunc:      upgradePersitentQuery,
				},
				{
					name: "Upgrade CORS",
					description: "This updates GraphQL schema to contain the CORS information. " +
						"Some of the dgraph internal predicates are removed in v21.03.0. " +
						"dgraph.cors that used to store CORS information is one of them. " +
						"For more info, see: https://github.com/dgraph-io/dgraph/pull/7431",
					minFromVersion: &version{major: 20, minor: 11, patch: 0},
					applyFunc:      upgradeCORS,
				},
			},
		},
	}
}
