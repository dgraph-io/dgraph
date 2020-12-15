+++
date = "2017-03-20T22:25:17+11:00"
title = "Upserts"
weight = 9
[menu.main]
    parent = "howto"
+++

Upsert-style operations are operations where:

1. A node is searched for, and then
2. Depending on if it is found or not, either:
    - Updating some of its attributes, or
    - Creating a new node with those attributes.

The upsert has to be an atomic operation such that either a new node is
created, or an existing node is modified. It's not allowed that two concurrent
upserts both create a new node.

There are many examples where upserts are useful. Most examples involve the
creation of a 1 to 1 mapping between two different entities. E.g. associating
email addresses with user accounts.

Upserts are common in both traditional RDBMSs and newer NoSQL databases.
Dgraph is no exception.

## Upsert Procedure

In Dgraph, upsert-style behaviour can be implemented by users on top of
transactions. The steps are as follows:

1. Create a new transaction.

2. Query for the node. This will usually be as simple as `{ q(func: eq(email,
   "bob@example.com")) { uid }}`. If a `uid` result is returned, then that's the
`uid` for the existing node. If no results are returned, then the user account
doesn't exist.

3. In the case where the user account doesn't exist, then a new node has to be
   created. This is done in the usual way by making a mutation (inside the
transaction), e.g.  the RDF `_:newAccount <email> "bob@example.com" .`. The
`uid` assigned can be accessed by looking up the blank node name `newAccount`
in the `Assigned` object returned from the mutation.

4. Now that you have the `uid` of the account (either new or existing), you can
   modify the account (using additional mutations) or perform queries on it in
whichever way you wish.

## Upsert Block

You can also use the `Upsert Block` to achieve the upsert procedure in a single
 mutation. The request will contain both the query and the mutation as explained
[here]({{< relref "mutations/upsert-block.md" >}}).

## Conflicts

Upsert operations are intended to be run concurrently, as per the needs of the
application. As such, it's possible that two concurrently running operations
could try to add the same node at the same time. For example, both try to add a
user with the same email address. If they do, then one of the transactions will
fail with an error indicating that the transaction was aborted.

If this happens, the transaction is rolled back and it's up to the user's
application logic to retry the whole operation. The transaction has to be
retried in its entirety, all the way from creating a new transaction.

The choice of index placed on the predicate is important for performance.
**Hash is almost always the best choice of index for equality checking.**

{{% notice "note" %}}
It's the _index_ that typically causes upsert conflicts to occur. The index is
stored as many key/value pairs, where each key is a combination of the
predicate name and some function of the predicate value (e.g. its hash for the
hash index). If two transactions modify the same key concurrently, then one
will fail.
{{% /notice %}}