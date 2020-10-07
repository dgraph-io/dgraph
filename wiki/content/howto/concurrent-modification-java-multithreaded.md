+++
date = "2017-09-10T22:25:17+11:00"
title = "Concurrent mutations and conflicts"
weight = 12
[menu.main]
    parent = "howto"
+++

This how-to guide provides an example on how to handle concurrent modifications using a multi-threaded Java Program. The example demonstrates [transaction]({{< relref "clients/overview.md#transactions" >}}) conflicts in Dgraph.

Steps to run this example are as follows.

Step 1: Start a new terminal and launch Dgraph with the following command line.
```sh
docker run -it -p 8080:8080 -p 9080:9080 dgraph/standalone:master
```
Step 2: Checkout the source code from the 'samples' directory in dgraph4j repository (https://github.com/dgraph-io/dgraph4j). This particular example can found at the path "samples/concurrent-modification". In order to run this example, execute the following maven command from the 'concurrent-modification' folder.
```sh
mvn clean install exec:java
```
Step 3: On running the example, the program initializes Dgraph with the following schema.
```sh
<clickCount>: int @index(int) .
<name>: string @index(exact) .
```
Step 4: The program also initializes user "Alice" with a 'clickCount' of value '1', and then proceeds to increment 'clickCount' concurrently in two threads. Dgraph throws an exception if a transaction is updating a given predicate that is being concurrently modified. As part of the exception handling logic, the program sleeps for 1 second on receiving a concurrent modification exception (“TxnConflictException”), and then retries. 
<br> The logs below show that two threads are increasing clickCount for the same user named Alice (note the same uid). Thread #1 succeeds immediately, and Dgraph throws a concurrent modification conflict on Thread 2. Thread 2 sleeps for 1 second and retries, and this time succeeds.
 
<-timestamp-> <-log->
```sh
1599628015260 Thread #2 increasing clickCount for uid 0xe, Name: Alice
1599628015260 Thread #1 increasing clickCount for uid 0xe, Name: Alice
1599628015291 Thread #1 succeeded after 0 retries
1599628015297 Thread #2 found a concurrent modification conflict, sleeping for 1 second...
1599628016297 Thread #2 resuming
1599628016310 Thread #2 increasing clickCount for uid 0xe, Name: Alice
1599628016333 Thread #2 succeeded after 1 retries
```
Step 5: Please note that the final value of clickCount is 3 (initial value was 1), which is correct. 
Query:
```json
{
  Alice(func: has(<name>)) @filter(eq(name,"Alice" )) {        
    uid
    name
    clickCount
  }
}
```
Response:
```json
{
  "data": {
    "Alice": [
      {
        "uid": "0xe",
        "name": "Alice",
        "clickCount": 3
      }
    ]
  }
}
```

***Summary***

Concurrent modifications to the same predicate causes the "TxnConflictException" exception. When several transactions hit the same node's predicate at the same time, the first one succeeds, while the other will get the “TxnConflictException”. Upon constantly retrying, the transactions begin to succeed one after another, and given enough retries, correctly completes its work.
