+++
date = "2017-03-20T22:25:17+11:00"
title = "Access Control Lists"
[menu.main]
    parent = "enterprise-features"
    weight = 2
+++

{{% notice "note" %}}
This feature was introduced in [v1.1.0](https://github.com/dgraph-io/dgraph/releases/tag/v1.1.0).
The Dgraph ACL tool is deprecated and would be removed in the next release. ACL changes can be made by using the `/admin` GraphQL endpoint on any Alpha node.
{{% /notice %}}

Access Control List (ACL) provides access protection to your data stored in
Dgraph. When the ACL feature is turned on, a client, e.g. dgo or dgraph4j, must
authenticate with a username and password before executing any transactions, and
is only allowed to access the data permitted by the ACL rules.

This document has two parts: first we will talk about the admin operations
needed for setting up ACL; then we will explain how to use a client to access
the data protected by ACL rules.

## Turn on ACLs

The ACL Feature can be turned on by following these steps

1. Since ACL is an enterprise feature, make sure your use case is covered under
a contract with Dgraph Labs Inc. You can contact us by sending an email to
[contact@dgraph.io](mailto:contact@dgraph.io) or post your request at [our discuss
forum](https://discuss.dgraph.io) to get an enterprise license.

2. Create a plain text file, and store a randomly generated secret key in it. The secret
key is used by Alpha servers to sign JSON Web Tokens (JWT). As you’ve probably guessed,
it’s critical to keep the secret key as a secret. Another requirement for the secret key
is that it must have at least 256-bits, i.e. 32 ASCII characters, as we are using
HMAC-SHA256 as the signing algorithm.

3. Start all the alpha servers in your cluster with the option `--acl_secret_file`, and
make sure they are all using the same secret key file created in Step 2.

Here is an example that starts one zero server and one alpha server with the ACL feature turned on:

```bash
dgraph zero --my=localhost:5080 --replicas 1 --idx 1 --bindall --expose_trace --profile_mode block --block_rate 10 --logtostderr -v=2
dgraph alpha --my=localhost:7080 --lru_mb=1024 --zero=localhost:5080 --logtostderr -v=3 --acl_secret_file ./hmac-secret
```

If you are using docker-compose, a sample cluster can be set up by:

1. `cd $GOPATH/src/github.com/dgraph-io/dgraph/compose/`
2. `make`
3. `./compose -e --acl_secret <path to your hmac secret file>`, after which a `docker-compose.yml` file will be generated.
4. `docker-compose up` to start the cluster using the `docker-compose.yml` generated above.

## Set up ACL Rules

Now that your cluster is running with the ACL feature turned on, you can set up the ACL rules. This can be done using the web UI Ratel or by using a GraphQL tool which fires the mutations. Execute the following mutations using a GraphQL tool like Insomnia, GraphQL Playground or GraphiQL.

A typical workflow is the following:

1. Reset the root password
2. Create a regular user
3. Create a group
4. Assign the user to the group
5. Assign predicate permissions to the group

### Using GraphQL Admin API
{{% notice "note" %}}
All these mutations require passing an `X-Dgraph-AccessToken` header, value for which can be obtained after logging in.
{{% /notice %}}

1) Reset the root password. The example below uses the dgraph endpoint `localhost:8080/admin`as a demo, make sure to choose the correct IP and port for your environment:
```graphql
mutation {
  updateUser(input: {filter: {name: {eq: "groot"}}, set: {password: "newpassword"}}) {
    user {
      name
    }
  }
}
```
The default password is `password`. `groot` is part of a special group called `guardians`. Members of `guardians` group will have access to everything. You can add more users to this group if required.

2) Create a regular user

```graphql
mutation {
  addUser(input: [{name: "alice", password: "newpassword"}]) {
    user {
      name
    }
  }
}
```

Now you should see the following output

```json
{
  "data": {
    "addUser": {
      "user": [
        {
          "name": "alice"
        }
      ]
    }
  }
}
```

3) Create a group

```graphql
mutation {
  addGroup(input: [{name: "dev"}]) {
    group {
      name
      users {
        name
      }
    }
  }
}
```

Now you should see the following output

```json
{
  "data": {
    "addGroup": {
      "group": [
        {
          "name": "dev",
          "users": []
        }
      ]
    }
  }
}
```

4) Assign the user to the group
To assign the user `alice` to both the group `dev` and the group `sre`, the mutation should be

```graphql
mutation {
  updateUser(input: {filter: {name: {eq: "alice"}}, set: {groups: [{name: "dev"}, {name: "sre"}]}}) {
    user {
      name
      groups {
        name
    }
    }
  }
}
```

5) Assign predicate permissions to the group

```graphql
mutation {
  updateGroup(input: {filter: {name: {eq: "dev"}}, set: {rules: [{predicate: "friend", permission: 7}]}}) {
    group {
      name
      rules {
        permission
        predicate
      }
    }
  }
}
```

Here we assigned a permission rule for the friend predicate to the group. In case you have [reverse edges]({{< relref "query-language/schema.md#reverse-edges" >}}), they have to be given the permission to the group as well
```graphql
mutation {
  updateGroup(input: {filter: {name: {eq: "dev"}}, set: {rules: [{predicate: "~friend", permission: 7}]}}) {
    group {
      name
      rules {
        permission
        predicate
      }
    }
  }
}
```
You can also resolve this by using the `dgraph acl` tool
```
dgraph acl -a <ALPHA_ADDRESS:PORT> -w <GROOT_USER> -x <GROOT_PASSWORD>  mod --group dev --pred ~friend --perm 7
```

The command above grants the `dev` group the `READ`+`WRITE`+`MODIFY` permission on the
`friend` predicate. Permissions are represented by a number following the UNIX file
permission convention. That is, 4 (binary 100) represents `READ`, 2 (binary 010)
represents `WRITE`, and 1 (binary 001) represents `MODIFY` (the permission to change a
predicate's schema). Similarly, permisson numbers can be bitwise OR-ed to represent
multiple permissions. For example, 7 (binary 111) represents all of `READ`, `WRITE` and
`MODIFY`. In order for the example in the next section to work, we also need to grant
full permissions on another predicate `name` to the group `dev`. If there are no rules for
a predicate, the default behavior is to block all (`READ`, `WRITE` and `MODIFY`) operations.

```graphql
mutation {
  updateGroup(input: {filter: {name: {eq: "dev"}}, set: {rules: [{predicate: "name", permission: 7}]}}) {
    group {
      name
      rules {
        permission
        predicate
      }
    }
  }
}
```

## Retrieve Users and Groups Information
{{% notice "note" %}}
All these queries require passing an `X-Dgraph-AccessToken` header, value for which can be obtained after logging in.
{{% /notice %}}
The following examples show how to retrieve information about users and groups.

### Using a GraphQL tool

1) Check information about a user

```graphql
query {
  getUser(name: "alice") {
    name
    groups {
      name
    }
  }
}
```

and the output should show the groups that the user has been added to, e.g.

```json
{
  "data": {
    "getUser": {
      "name": "alice",
      "groups": [
        {
          "name": "dev"
        }
      ]
    }
  }
}
```

2) Check information about a group

```graphql
{
  getGroup(name: "dev") {
    name
    users {
      name
    }
    rules {
      permission
      predicate
    }
  }
}
```

and the output should include the users in the group, as well as the permissions, the
group's ACL rules, e.g.

```json
{
  "data": {
    "getGroup": {
      "name": "dev",
      "users": [
        {
          "name": "alice"
        }
      ],
      "rules": [
        {
          "permission": 7,
          "predicate": "friend"
        },
        {
          "permission": 7,
          "predicate": "name"
        }
      ]
    }
  }
}
```

3) Query for users

```graphql
query {
  queryUser(filter: {name: {eq: "alice"}}) {
    name
    groups {
      name
    }
  }
}
```

and the output should show the groups that the user has been added to, e.g.

```json
{
  "data": {
    "queryUser": [
      {
        "name": "alice",
        "groups": [
          {
            "name": "dev"
          }
        ]
      }
    ]
  }
}
```

4) Query for groups

```graphql
query {
  queryGroup(filter: {name: {eq: "dev"}}) {
    name
    users {
      name
    }
    rules {
      permission
      predicate
    }
  }
}
```

and the output should include the users in the group, as well as the permissions the
group's ACL rules, e.g.

```json
{
  "data": {
    "queryGroup": [
      {
        "name": "dev",
        "users": [
          {
            "name": "alice"
          }
        ],
        "rules": [
          {
            "permission": 7,
            "predicate": "friend"
          },
          {
            "permission": 7,
            "predicate": "name"
          }
        ]
      }
    ]
  }
}
```

5) Run ACL commands as another guardian (member of `guardians` group).

You can also run ACL commands with other users. Say we have a user `alice` which is member
of `guardians` group and its password is `simple_alice`.

## Access Data Using a Client

Now that the ACL data are set, to access the data protected by ACL rules, we need to
first log in through a user. This is tyically done via the client's `.login(USER_ID, USER_PASSWORD)` method.

A sample code using the dgo client can be found
[here](https://github.com/dgraph-io/dgraph/blob/master/tlstest/acl/acl_over_tls_test.go). An example using
dgraph4j can be found [here](https://github.com/dgraph-io/dgraph4j/blob/master/src/test/java/io/dgraph/AclTest.java).

## Access Data Using the GraphQL API

Dgraph's HTTP API also supports authenticated operations to access ACL-protected
data.

To login, send a POST request to `/admin` with the GraphQL mutation. For example, to log in as the root user groot:

```graphql
mutation {
  login(userId: "groot", password: "password") {
    response {
      accessJWT
      refreshJWT
    }
  }
}
```

Response:

```json
{
  "data": {
    "accessJWT": "<accessJWT>",
    "refreshJWT": "<refreshJWT>"
  }
}
```

The response includes the access and refresh JWTs which are used for the authentication itself and refreshing the authentication token, respectively. Save the JWTs from the response for later HTTP requests.

You can run authenticated requests by passing the accessJWT to a request via the `X-Dgraph-AccessToken` header. Add the header `X-Dgraph-AccessToken` with the `accessJWT` value which you got in the login response in the GraphQL tool which you're using to make the request. For example:

```graphql
mutation {
  addUser(input: [{name: "alice", password: "newpassword"}]) {
    user {
      name
    }
  }
}
```

The refresh token can be used in the `/admin` POST GraphQL mutation to receive new access and refresh JWTs, which is useful to renew the authenticated session once the ACL access TTL expires (controlled by Dgraph Alpha's flag `--acl_access_ttl` which is set to 6h0m0s by default).

```graphql
mutation {
  login(userId: "groot", password: "newpassword", refreshToken: "<refreshJWT>") {
    response {
      accessJWT
      refreshJWT
    }
  }
}
```

## Reset Groot Password

If you've forgotten the password to your groot user, then you may reset the groot password (or
the password for any user) by following these steps.

1. Stop Dgraph Alpha.
2. Turn off ACLs by removing the `--acl_hmac_secret` config flag in the Alpha config. This leaves
   the Alpha open with no ACL rules, so be sure to restrict access, including stopping request
   traffic to this Alpha.
3. Start Dgraph Alpha.
4. Connect to Dgraph Alpha using Ratel and run the following upsert mutation to update the groot password
   to `newpassword` (choose your own secure password):
   ```text
   upsert {
     query {
       groot as var(func: eq(dgraph.xid, "groot"))
     }
     mutation {
       set {
         uid(groot) <dgraph.password> "newpassword" .
       }
     }
   }
   ```
5. Restart Dgraph Alpha with ACLs turned on by setting the `--acl_hmac_secret` config flag.
6. Login as groot with your new password.