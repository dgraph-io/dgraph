// +build !oss

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

package admin

const adminTypes = `
	input BackupInput {
		destination: String!
		accessKey: String
		secretKey: String
		sessionToken: String
		anonymous: Boolean
		forceFull: Boolean
	}

	type BackupPayload {
		response: Response
	}

	type LoginResponse {
		accessJWT: String
		refreshJWT: String
	}

	type LoginPayload {
		response: LoginResponse
	}

	type User @secret(field: "password", pred: "dgraph.password") {
		name: String! @id @dgraph(pred: "dgraph.xid")
		groups: [Group] @dgraph(pred: "dgraph.user.group")
	}

	type Group {
		name: String! @id @dgraph(pred: "dgraph.xid")
		users: [User] @dgraph(pred: "~dgraph.user.group")
		rules: [Rule] @dgraph(pred: "dgraph.acl.rule")
	}

	type Rule {
		id: ID!
		predicate: String! @dgraph(pred: "dgraph.rule.predicate")
		# TODO - Change permission to enum type once we figure out how to map enum strings to Int
		# while storing it in Dgraph.
		# We also need validation in Dgraph on ther permitted value of permission to be between [0,7]
		# If we change permission to be an ENUM and only allow ACL mutations through the GraphQL API
		# then we don't need this validation in Dgrpah.
		permission: Int! @dgraph(pred: "dgraph.rule.permission")
	}

	input StringHashFilter {
		eq: String
	}

	enum UserOrderable {
		name
	}

	enum GroupOrderable {
		name
	}

	input AddUserInput {
		name: String!
		password: String!
		groups: [GroupRef]
	}

	input AddGroupInput {
		name: String!
		rules: [RuleRef]
	}

	input UserRef {
		name: String!
	}

	input GroupRef {
		name: String!
	}

	input RuleRef {
		id: ID
		predicate: String
		permission: Int
	}

	input UserFilter {
		name: StringHashFilter
		and: UserFilter
		or: UserFilter
		not: UserFilter
	}

	input UserOrder {
		asc: UserOrderable
		desc: UserOrderable
		then: UserOrder
	}

	input GroupOrder {
		asc: GroupOrderable
		desc: GroupOrderable
		then: GroupOrder
	}

	input UserPatch {
		password: String
		groups: [GroupRef]
	}

	input UpdateUserInput {
		filter: UserFilter!
		set: UserPatch
		remove: UserPatch
	}

	input GroupFilter {
		name: StringHashFilter
		and: UserFilter
		or: UserFilter
		not: UserFilter
	}

	input GroupPatch {
		rules: [RuleRef]
	}

	input UpdateGroupInput {
		filter: GroupFilter!
		set: GroupPatch
		remove: GroupPatch
	}

	type AddUserPayload {
		user: [User]
	}

	type AddGroupPayload {
		group: [Group]
	}

	type DeleteUserPayload {
		msg: String
	}

	type DeleteGroupPayload {
		msg: String
	}`

const adminMutations = `
	backup(input: BackupInput!) : BackupPayload

	login(userId: String, password: String, refreshToken: String): LoginPayload
	# ACL related endpoints.
	# 1. If user and group don't exist both are created and linked.
	# 2. If user doesn't exist but group does, then user is created and both are linked.
	# 3. If user exists and group doesn't exist, then two errors are returned i.e. User exists
	# and group doesn't exist.
	# 4. If user and group exists, then error that user exists.
	addUser(input: [AddUserInput]): AddUserPayload
	addGroup(input: [AddGroupInput]): AddGroupPayload

	# update user allows updating a user's password or updating their groups. If the group
	# doesn't exist, then it is created, otherwise linked to the user. If the user filter
	# doesn't return anything then nothing happens.
	updateUser(input: UpdateUserInput!): AddUserPayload
	# update group only allows adding rules to a group.
	updateGroup(input: UpdateGroupInput!): AddGroupPayload

	deleteGroup(filter: GroupFilter!): DeleteGroupPayload
	deleteUser(filter: UserFilter!): DeleteUserPayload`

const adminQueries = `
	# ACL related endpoints
	# TODO - The endpoints below work fine for members of guardians group but they should only
	# return a subset of the data for other users. Test that and add validation in the server
	# for them.
	getUser(name: String!): User
	getGroup(name: String!): Group

	getCurrentUser: User

	queryUser(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
	queryGroup(filter: GroupFilter, order: GroupOrder, first: Int, offset: Int): [Group]`
