# Dgraph Design Doc

To explain Dgraph working, let's start with an example query we should be able
to run.

Query to answer:
Find all posts liked by friends of friends over the last year, written by a popular author X.

In a SQL / NoSQL database, this would require you to retrieve a lot of data:

Method 1:

* Find all the friends (~ 200 friends)
* Find all their friends (~ 200 * 200 = 40,000 people)
* Find all the posts liked by these people (resulting set in millions).
* Intersect these posts with posts authored by person X.

Method 2:

* Find all posts written by person X over the last year (possibly thousands).
* Find all people who liked those posts (easily millions) = result set 1.
* Find all your friends
* Find all their friends = result set 2.
* Intersect result set 1 with result set 2.

Both of these approaches, would result in a lot of data going
back and forth between database
and application. Lots of network calls.
Or, would require you to run a MapReduce.

This distributed graph serving system is designed to make queries like these
run in production, with production level latency.

This is how it would run:

* Node X contains posting list for predicate `friends`
* Seek to caller's userid in Node X (1 RPC). Retrieve a list of friend uids.
* Do multiple seeks for each of the friend uids, to generate a list of
  friends of friends uids. **[result set 1]**
* Node Y contains posting list for predicate `posts_liked`.
* Ship result set 1 to Node Y (1 RPC), and do seeks to generate a list
	of all posts liked by result set 1. **[result set 2]**
* Node Z contains posting list for predicate `author`.
* Ship result set 2 to Node Z (1 RPC). Seek to author X, and generate a list of
  posts authored by X. **[result set 3]**
* Intersect the two sorted lists, result set 2 and result set 3. **[result set 4]**
* Node N contains names for all uids.
* Ship result set 4 to Node N (1 RPC),
and convert uids to names by doing multiple seeks. **[result set 5]**
* Ship result set 5 back to caller.

In 4-5 RPCs, we have figured out all the posts liked by friends of friends,
authored by person X.

What if a predicate's posting list is too big to fully store on 1 node?

It can be split sharded to fit on multiple machines,
just like Bigtable sharding works. This would mean a few additional RPCs, but
still no where near as many as would be required by existing datastores.

This system would allow vast scalability, and yet production level latencies,
to support running complicated queries requiring deep joins.
