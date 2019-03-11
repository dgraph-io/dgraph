+++
date = "Fri Jan 18 16:07:43 PST 2019"
title = "Access Control List"
+++

Access Control List (ACL) provides protection to your data stored in
Dgraph. When the ACL feature is turned on, a client, e.g. dgo or dgraph4j, must login first using a
pair of username and password before executing any transactions, and is only allowed to access the
data permitted by the ACL rules. This document has two parts: first we will talk about
the admin operations needed for setting up ACL;  then we will explain how to use a client to
access the data protected by ACL rules. 

## Turn on ACL in your cluster
The ACL Feature can be turned on by following these steps 

1. Since ACL is one of our enterprise features, make sure your use case is covered under a contract with Dgraph Labs Inc. 
You can contact us by sending an email to [contact@dgraph.io](mailto:contact@dgraph.io) or post your request at [our discuss
forum](https://discuss.dgraph.io).

2. Create a plain text file, and store a randomly generated secret key in it. The secret key is used
by Alpha servers to sign JSON Web Tokens (JWT). As you’ve probably guessed, it’s critical to keep
the secret key as a secret. Another requirement for the secret key is that it must have at least 256
bits, i.e.  32 ASCII characters, as we are using HMAC-SHA256 as the signing algorithm,.

3. Start all the alpha servers in your cluster with the
options `--enterprise_features` and `--acl_secret_file`, and make sure they are all using the same
secret key file created in Step 2. 

Here is an example that starts one zero server and one alpha server with the ACL feature turned on:
```
dgraph zero --my=localhost:5080 --replicas 1 --idx 1 --bindall --expose_trace --profile_mode block --block_rate 10 --logtostderr -v=2
dgraph alpha --my=localhost:7080 --lru_mb=1024 --zero=localhost:5080 --logtostderr -v=3 --acl_secret_file ./hmac-secret --enterprise_features
```

If you are using docker-compose, a sample cluster can be set up by:

1. `cd $GOPATH/src/github.com/dgraph-io/dgraph/compose/`

2. `make`

3. `./compose -e --acl_secret <path to your hmac secret file>`, after which a `docker-compose.yml` file will be generated.

4. `docker-compose up` to start the cluster using the `docker-compose.yml` generated above.

## Set up ACL rules
Now that your cluster is running with the ACL feature turned on, let's set up the ACL rules. A typical workflow is the following:

1. reset the root password. The example below uses the dgraph endpoint `localhost:9180` as a demo, make sure to choose the correct one for your environment:
```bash
dgraph acl passwd -d localhost:9180 -u groot
```
Now type in the password for the groot account, which is the superuser that has access to everything. If you have never changed the groot password, by default it's `password`.

2. create a regular user
```bash
dgraph acl useradd -u alice -p simplepassword -d localhost:9180
```
Now you should be able to see the following output
```bash
    Current password for groot:

    Running transaction with dgraph endpoint: localhost:9180
    Login successful.
    Created new user with id alice
```

3. create a group
```bash
dgraph acl groupadd -d localhost:9180 -g dev
```
Again type in the groot password, and you should see the following output
```bash
    Current password for groot:
    
    Running transaction with dgraph endpoint: localhost:9180
    Login successful.
    Created new group with id dev
```
4. assign the user to the group
```bash
dgraph acl usermod -d localhost:9180 -u alice -g dev
```
The command above will add `alice` to the `dev` group. A user can be assigned to multiple groups. 
The multiple groups should be formated as a single string separated by `,`.
For example, to assign the user `alice` to both the group `dev` and the group `sre`, the command should be
```bash
dgraph acl usermod -d localhost:9180 -u alice -g dev,sre
```
5. assign predicate permissions to the group
```bash
dgraph acl chmod -d localhost:9180 -g dev -p friend -P 7
```
The command above grants the `dev` group with the `READ`+`WRITE`+`MODIFY` permisson on the `friend` predicate. Permissions are represented by a number following the UNIX file permission convention. 
That is, 4 (binary 100) represents `READ`, 2 (binary 010) represents `WRITE`, and 1 (binary 001) represents `MODIFY` (the permission to change a predicate's schema). Similarly, permisson numbers can be XORed to represent multiple permissions. For example, 7 (binary 111) represents all of `READ`, `WRITE` and `MODIFY`.
In order for the example in the next section to work, we also need to grant full permissions on another predicate `name` to the group `dev`
```bash
dgraph acl chmod -d localhost:9180 -g dev -p name -P 7
```

6. check information about a user
```bash
dgraph acl info -d localhost:9180 -u alice
```
and the output should show the groups that the user has been added to, e.g.
```bash
I0121 14:19:20.584620   14095 run_ee.go:194] Running transaction with dgraph endpoint: localhost:9180
I0121 14:19:20.697279   14095 utils.go:172] login successfully to the groot account
I0121 14:19:20.705194   14095 run_ee.go:249] user alice:
uid:0x3
id:alice
groups:dev
```
7. check information about a group
```bash
dgraph acl info -d localhost:9180 -g dev
```
and the output should include the users in the group, as well as the permissions that the group has access to, e.g.
```bash
Enter groot password:I0125 11:34:46.710795   10401 run_ee.go:194] Running transaction with dgraph endpoint: localhost:9180
I0125 11:34:46.846839   10401 utils.go:172] login successfully to the groot account
I0125 11:34:46.860949   10401 run_ee.go:281] group dev:
uid:0x4
id:dev
users:alice
acls:(predicate:friend,perm:7) (predicate:name,perm:7)
```

## Access data using a client
Now that ACL rules are set, let's store some data in Dgraph and run queries to retrieve them back using the dgo client.
[Place Holder Example using dgo client](http://www.dgraph.io)

A similar example using the dgraph4j client is [Place holder here](http://www.dgraph.io)

## Implementation details
If you are curious about how we implemented ACL, you can find the details [here](./impl)
