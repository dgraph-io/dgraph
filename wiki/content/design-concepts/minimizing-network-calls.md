+++
date = "2017-03-20T22:25:17+11:00"
title = "Minimizing network calls explained"
weight = 4
[menu.main]
    parent = "design-concepts"
+++

To explain how Dgraph minimizes network calls, let's start with an example query we should be able
to run.

*Find all posts liked by friends of friends of mine over the last year, written by a popular author X.*

## SQL/NoSQL
In a distributed SQL/NoSQL database, this would require you to retrieve a lot of data.

Method 1:

* Find all the friends (~ 338 [friends](http://www.pewresearch.org/fact-tank/2014/02/03/6-new-facts-about-facebook/</ref>)).
* Find all their friends (~ 338 * 338 = 40,000 people).
* Find all the posts liked by these people over the last year (resulting set in millions).
* Intersect these posts with posts authored by person X.

Method 2:

* Find all posts written by popular author X over the last year (possibly thousands).
* Find all people who liked those posts (easily millions) `result set 1`.
* Find all your friends.
* Find all their friends `result set 2`.
* Intersect `result set 1` with `result set 2`.

Both of these approaches would result in a lot of data going back and forth between database and
application; would be slow to execute, or would require you to run an offline job.

## Dgraph
This is how it would run in Dgraph:

* Node X contains posting list for predicate `friends`.
* Seek to caller's userid in Node X **(1 RPC)**. Retrieve a list of friend uids.
* Do multiple seeks for each of the friend uids, to generate a list of friends of friends uids. `result set 1`
* Node Y contains posting list for predicate `posts_liked`.
* Ship result set 1 to Node Y **(1 RPC)**, and do seeks to generate a list of all posts liked by
result set 1. `result set 2`
* Node Z contains posting list for predicate `author`.
* Ship result set 2 to Node Z **(1 RPC)**. Seek to author X, and generate a list of posts authored
by X. `result set 3`
* Intersect the two sorted lists, `result set 2` and `result set 3`. `result set 4`
* Node N contains names for all uids.
* Ship `result set 4` to Node N **(1 RPC)**, and convert uids to names by doing multiple seeks. `result set 5`
* Ship `result set 5` back to caller.

In 4-5 RPCs, we have figured out all the posts liked by friends of friends, written by popular author X.

This design allows vast scalability, and yet consistent production level latencies,
to support running complicated queries requiring deep joins.