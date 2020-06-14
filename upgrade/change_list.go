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
				{
					name: "Upgrade non-predefined types/predicates using reserved namespace",
					description: "This updates any user-defined types/predicates which start with" +
						" `dgraph.` to use a name which doesn't start with `dgraph.`. The user " +
						"will have to provide a new name to use for such types/predicates. " +
						"This change was introduced in v20.07.0, " +
						"and is applied while upgrading from any version less than v20.07.0+. " +
						"For more info, see: https://github.com/dgraph-io/dgraph/pull/5185",
					minFromVersion: zeroVersion,
					applyFunc:      upgradeNonPredefinedNamesInReservedNamespace,
				},
			},
		},
	}
}
