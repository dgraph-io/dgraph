// +build !oss

/*
 * Copyright 2020 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package admin

const adminTypes = `

	type LoginResponse {

		"""
		JWT token that should be used in future requests after this login.
		"""
		accessJWT: String

		"""
		Refresh token that can be used to re-login after accessJWT expires.
		"""
		refreshJWT: String
	}

	type LoginPayload {
		response: LoginResponse
	}

	type User @dgraph(type: "dgraph.type.User") @secret(field: "password", pred: "dgraph.password") {

		"""
		Username for the user.  Dgraph ensures that usernames are unique.
		"""
		name: String! @id @dgraph(pred: "dgraph.xid")

		groups: [Group] @dgraph(pred: "dgraph.user.group")
	}

	type Group @dgraph(type: "dgraph.type.Group") {

		"""
		Name of the group.  Dgraph ensures uniqueness of group names.
		"""
		name: String! @id @dgraph(pred: "dgraph.xid")
		users: [User] @dgraph(pred: "~dgraph.user.group")
		rules: [Rule] @dgraph(pred: "dgraph.acl.rule")
	}

	type Rule @dgraph(type: "dgraph.type.Rule") {

		"""
		Predicate to which the rule applies.
		"""
		predicate: String! @dgraph(pred: "dgraph.rule.predicate")

		"""
		Permissions that apply for the rule.  Represented following the UNIX file permission
		convention. That is, 4 (binary 100) represents READ, 2 (binary 010) represents WRITE,
		and 1 (binary 001) represents MODIFY (the permission to change a predicate’s schema).

		The options are:
		* 1 (binary 001) : MODIFY
		* 2 (010) : WRITE
		* 3 (011) : WRITE+MODIFY
		* 4 (100) : READ
		* 5 (101) : READ+MODIFY
		* 6 (110) : READ+WRITE
		* 7 (111) : READ+WRITE+MODIFY

		Permission 0, which is equal to no permission for a predicate, blocks all read,
		write and modify operations.
		"""
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
		"""
		Predicate to which the rule applies.
		"""
		predicate: String!

		"""
		Permissions that apply for the rule.  Represented following the UNIX file permission
		convention. That is, 4 (binary 100) represents READ, 2 (binary 010) represents WRITE,
		and 1 (binary 001) represents MODIFY (the permission to change a predicate’s schema).

		The options are:
		* 1 (binary 001) : MODIFY
		* 2 (010) : WRITE
		* 3 (011) : WRITE+MODIFY
		* 4 (100) : READ
		* 5 (101) : READ+MODIFY
		* 6 (110) : READ+WRITE
		* 7 (111) : READ+WRITE+MODIFY

		Permission 0, which is equal to no permission for a predicate, blocks all read,
		write and modify operations.
		"""
		permission: Int!
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

	input SetGroupPatch {
		rules: [RuleRef!]!
	}

	input RemoveGroupPatch {
		rules: [String!]!
	}

	input UpdateGroupInput {
		filter: GroupFilter!
		set: SetGroupPatch
		remove: RemoveGroupPatch
	}

	type AddUserPayload {
		user: [User]
	}

	type AddGroupPayload {
		group: [Group]
	}

	type DeleteUserPayload {
		msg: String
		numUids: Int
	}

	type DeleteGroupPayload {
		msg: String
		numUids: Int
	}

	input AddNamespaceInput {
		password: String
	}

	input DeleteNamespaceInput {
		namespaceId: Int!
	}

	type NamespacePayload {
		namespaceId: UInt64
		message: String
	}

	input ResetPasswordInput {
		userId: String!
		password: String!
		namespace: Int!
	}

	type ResetPasswordPayload {
		userId: String
		message: String
		namespace: UInt64
	}

	input EnterpriseLicenseInput {
		"""
		The contents of license file as a String.
		"""
		license: String!
	}

	type EnterpriseLicensePayload {
		response: Response
	}
	`

const adminMutations = `


	"""
	Login to Dgraph.  Successful login results in a JWT that can be used in future requests.
	If login is not successful an error is returned.
	"""
	login(userId: String, password: String, namespace: Int, refreshToken: String): LoginPayload

	"""
	Add a user.  When linking to groups: if the group doesn't exist it is created; if the group
	exists, the new user is linked to the existing group.  It's possible to both create new
	groups and link to existing groups in the one mutation.

	Dgraph ensures that usernames are unique, hence attempting to add an existing user results
	in an error.
	"""
	addUser(input: [AddUserInput!]!): AddUserPayload

	"""
	Add a new group and (optionally) set the rules for the group.
	"""
	addGroup(input: [AddGroupInput!]!): AddGroupPayload

	"""
	Update users, their passwords and groups.  As with AddUser, when linking to groups: if the
	group doesn't exist it is created; if the group exists, the new user is linked to the existing
	group.  If the filter doesn't match any users, the mutation has no effect.
	"""
	updateUser(input: UpdateUserInput!): AddUserPayload

	"""
	Add or remove rules for groups. If the filter doesn't match any groups,
	the mutation has no effect.
	"""
	updateGroup(input: UpdateGroupInput!): AddGroupPayload

	deleteGroup(filter: GroupFilter!): DeleteGroupPayload
	deleteUser(filter: UserFilter!): DeleteUserPayload

	"""
	Add a new namespace.
	"""
	addNamespace(input: AddNamespaceInput): NamespacePayload

	"""
	Delete a namespace.
	"""
	deleteNamespace(input: DeleteNamespaceInput!): NamespacePayload

	"""
	Reset password can only be used by the Guardians of the galaxy to reset password of
	any user in any namespace.
	"""
	resetPassword(input: ResetPasswordInput!): ResetPasswordPayload

	"""
	Apply enterprise license.
	"""
	enterpriseLicense(input: EnterpriseLicenseInput!): EnterpriseLicensePayload
	`

const adminQueries = `
	getUser(name: String!): User
	getGroup(name: String!): Group

	"""
	Get the currently logged in user.
	"""
	getCurrentUser: User

	queryUser(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
	queryGroup(filter: GroupFilter, order: GroupOrder, first: Int, offset: Int): [Group]

	`
