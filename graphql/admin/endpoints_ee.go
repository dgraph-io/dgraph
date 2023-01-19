//go:build !oss
// +build !oss

/*
 * Copyright 2022 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package admin

const adminTypes = `
	input BackupInput {

		"""
		Destination for the backup: e.g. Minio or S3 bucket.
		"""
		destination: String!

		"""
		Access key credential for the destination.
		"""
		accessKey: String

		"""
		Secret key credential for the destination.
		"""
		secretKey: String

		"""
		AWS session token, if required.
		"""
		sessionToken: String

		"""
		Set to true to allow backing up to S3 or Minio bucket that requires no credentials.
		"""
		anonymous: Boolean

		"""
		Force a full backup instead of an incremental backup.
		"""
		forceFull: Boolean
	}

	type BackupPayload {
		response: Response
		taskId: String
	}

	input RestoreInput {

		"""
		Destination for the backup: e.g. Minio or S3 bucket.
		"""
		location: String!

		"""
		Backup ID of the backup series to restore. This ID is included in the manifest.json file.
		If missing, it defaults to the latest series.
		"""
		backupId: String

		"""
		Number of the backup within the backup series to be restored. Backups with a greater value
		will be ignored. If the value is zero or missing, the entire series will be restored.
		"""
		backupNum: Int

		"""
		Path to the key file needed to decrypt the backup. This file should be accessible
		by all alphas in the group. The backup will be written using the encryption key
		with which the cluster was started, which might be different than this key.
		"""
		encryptionKeyFile: String

		"""
		Vault server address where the key is stored. This server must be accessible
		by all alphas in the group. Default "http://localhost:8200".
		"""
		vaultAddr: String

		"""
		Path to the Vault RoleID file.
		"""
		vaultRoleIDFile: String

		"""
		Path to the Vault SecretID file.
		"""
		vaultSecretIDFile: String

		"""
		Vault kv store path where the key lives. Default "secret/data/dgraph".
		"""
		vaultPath: String

		"""
		Vault kv store field whose value is the key. Default "enc_key".
		"""
		vaultField: String

		"""
		Vault kv store field's format. Must be "base64" or "raw". Default "base64".
		"""
		vaultFormat: String

		"""
		Access key credential for the destination.
		"""
		accessKey: String

		"""
		Secret key credential for the destination.
		"""
		secretKey: String

		"""
		AWS session token, if required.
		"""
		sessionToken: String

		"""
		Set to true to allow backing up to S3 or Minio bucket that requires no credentials.
		"""
		anonymous: Boolean
	}

	type RestorePayload {
		"""
		A short string indicating whether the restore operation was successfully scheduled.
		"""
		code: String

		"""
		Includes the error message if the operation failed.
		"""
		message: String
	}

	input ListBackupsInput {
		"""
		Destination for the backup: e.g. Minio or S3 bucket.
		"""
		location: String!

		"""
		Access key credential for the destination.
		"""
		accessKey: String

		"""
		Secret key credential for the destination.
		"""
		secretKey: String

		"""
		AWS session token, if required.
		"""
		sessionToken: String

		"""
		Whether the destination doesn't require credentials (e.g. S3 public bucket).
		"""
		anonymous: Boolean

	}

	type BackupGroup {
		"""
		The ID of the cluster group.
		"""
		groupId: UInt64

		"""
		List of predicates assigned to the group.
		"""
		predicates: [String]
	}

	type Manifest {
		"""
		Unique ID for the backup series.
		"""
		backupId: String

		"""
		Number of this backup within the backup series. The full backup always has a value of one.
		"""
		backupNum: UInt64

		"""
		Whether this backup was encrypted.
		"""
		encrypted: Boolean

		"""
		List of groups and the predicates they store in this backup.
		"""
		groups: [BackupGroup]

		"""
		Path to the manifest file.
		"""
		path: String

		"""
		The timestamp at which this backup was taken. The next incremental backup will
		start from this timestamp.
		"""
		since: UInt64

		"""
		The type of backup, either full or incremental.
		"""
		type: String
	}

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
		"""
		Enter a new password for groot in that namespace. If you leave it blank, the password will be the default.
		"""
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
	Start a binary backup.  See : https://dgraph.io/docs/enterprise-features/#binary-backups
	"""
	backup(input: BackupInput!) : BackupPayload

	"""
	Start restoring a binary backup.  See :
		https://dgraph.io/docs/enterprise-features/#binary-backups
	"""
	restore(input: RestoreInput!) : RestorePayload

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

	"""
	Get the information about the backups at a given location.
	"""
	listBackups(input: ListBackupsInput!) : [Manifest]
	`
