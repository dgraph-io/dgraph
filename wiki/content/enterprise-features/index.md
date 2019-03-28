+++
date = "2017-03-20T19:35:35+11:00"
title = "Enterprise Features"
+++

## Access Control Lists

Access Control List (ACL) provides access protection to your data stored in
Dgraph. When the ACL feature is turned on, a client, e.g. dgo or dgraph4j, must
authenticate with a username and password before executing any transactions, and
is only allowed to access the data permitted by the ACL rules.

This document has two parts: first we will talk about the admin operations
needed for setting up ACL; then we will explain how to use a client to access
the data protected by ACL rules.

### Turn on ACLs
The ACL Feature can be turned on by following these steps

1. Since ACL is one of our enterprise features, make sure your use case is covered under a contract with Dgraph Labs Inc.
You can contact us by sending an email to [contact@dgraph.io](mailto:contact@dgraph.io) or post your request at [our discuss
forum](https://discuss.dgraph.io).

2. Create a plain text file, and store a randomly generated secret key in it. The secret key is used
by Alpha servers to sign JSON Web Tokens (JWT). As you’ve probably guessed, it’s critical to keep
the secret key as a secret. Another requirement for the secret key is that it must have at least 256-bits, i.e. 32 ASCII characters, as we are using HMAC-SHA256 as the signing algorithm.

3. Start all the alpha servers in your cluster with the
options `--enterprise_features` and `--acl_secret_file`, and make sure they are all using the same
secret key file created in Step 2.

Here is an example that starts one zero server and one alpha server with the ACL feature turned on:
```
dgraph zero --my=localhost:5080 --replicas 1 --idx 1 --bindall --expose_trace --profile_mode block --block_rate 10 --logtostderr -v=2
dgraph alpha --enterprise_features --my=localhost:7080 --lru_mb=1024 --zero=localhost:5080 --logtostderr -v=3 --acl_secret_file ./hmac-secret
```

If you are using docker-compose, a sample cluster can be set up by:

1. `cd $GOPATH/src/github.com/dgraph-io/dgraph/compose/`

2. `make`

3. `./compose -e --acl_secret <path to your hmac secret file>`, after which a `docker-compose.yml` file will be generated.

4. `docker-compose up` to start the cluster using the `docker-compose.yml` generated above.

### Set up ACL rules

Now that your cluster is running with the ACL feature turned on, let's set up the ACL rules. A typical workflow is the following:

1. Reset the root password. The example below uses the dgraph endpoint `localhost:9180` as a demo, make sure to choose the correct one for your environment:
```bash
dgraph acl -d localhost:9180 mod -u groot
```
Now type in the password for the groot account, which is the superuser that has access to everything. The default password is `password`.

2. Create a regular user
```bash
dgraph acl -d localhost:9180 add -u alice
```
Now you should see the following output
```bash
Current password for groot:
Running transaction with dgraph endpoint: localhost:9180
Login successful.
New password for alice:
Retype new password for alice:
Created new user with id alice
```

3. Create a group
```bash
dgraph acl -d localhost:9180 add -g dev
```
Again type in the groot password, and you should see the following output
```bash
Current password for groot:
Running transaction with dgraph endpoint: localhost:9180
Login successful.
Created new group with id dev
```
4. Assign the user to the group
```bash
dgraph acl -d localhost:9180 mod -u alice -l dev
```
The command above will add `alice` to the `dev` group. A user can be assigned to multiple groups.
The multiple groups should be formated as a single string separated by `,`.
For example, to assign the user `alice` to both the group `dev` and the group `sre`, the command should be
```bash
dgraph acl -d localhost:9180 mod -u alice -l dev,sre
```
5. Assign predicate permissions to the group
```bash
dgraph acl mod -d localhost:9180 -g dev -p friend -P 7
```
The command above grants the `dev` group the `READ`+`WRITE`+`MODIFY` permission on the `friend` predicate. Permissions are represented by a number following the UNIX file permission convention.
That is, 4 (binary 100) represents `READ`, 2 (binary 010) represents `WRITE`, and 1 (binary 001) represents `MODIFY` (the permission to change a predicate's schema). Similarly, permisson numbers can be bitwise OR-ed to represent multiple permissions. For example, 7 (binary 111) represents all of `READ`, `WRITE` and `MODIFY`.
In order for the example in the next section to work, we also need to grant full permissions on another predicate `name` to the group `dev`
```bash
dgraph acl mod -d localhost:9180 -g dev -p name -P 7
```

6. Check information about a user
```bash
dgraph acl info -d localhost:9180 -u alice
```
and the output should show the groups that the user has been added to, e.g.
```bash
Running transaction with dgraph endpoint: localhost:9180
Login successful.
User  : alice
UID   : 0x3
Group : dev
Group : sre
```

7. Check information about a group
```bash
dgraph acl info -d localhost:9180 -g dev
```
and the output should include the users in the group, as well as the permissions the group's ACL rules, e.g.
```bash
Current password for groot:
Running transaction with dgraph endpoint: localhost:9180
Login successful.
Group: dev
UID  : 0x4
ID   : dev
Users: alice
ACL  : {friend  7}
ACL  : {name  7}
```

### Access data using a client

Now that the ACL data are set, to access the data protected by ACL rules, we need to first log in through a user.
In the dgo client, this is done through the `Login` method:
```go
	serviceAddr := "localhost:9180"
	conn, err := grpc.Dial(serviceAddr, grpc.WithInsecure())
	if err != nil {
		return err
	}

	ctx := context.Background()
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	if err := dg.Login(ctx, "alice", "password123"); err != nil {
		return err
	}

	txn := dg.NewTxn()
	defer txn.Discard(ctx)
	_, err = txn.Mutate(...)
```
