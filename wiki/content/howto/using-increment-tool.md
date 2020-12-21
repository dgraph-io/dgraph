+++
date = "2017-03-20T22:25:17+11:00"
title = "Using the Increment Tool"
weight = 4
[menu.main]
    parent = "howto"
+++

The `dgraph increment` tool increments a counter value transactionally. The
increment tool can be used as a health check that an Alpha is able to service
transactions for both queries and mutations.

## Example Usage

Increment the default predicate (`counter.val`) once. If the predicate doesn't yet
exist, then it will be created starting at counter 0.

```sh
$ dgraph increment
```

Increment the counter predicate against the Alpha running at address `--alpha` (default: `localhost:9080`):

```sh
$ dgraph increment --alpha=192.168.1.10:9080
```

Increment the counter predicate specified by `--pred` (default: `counter.val`):

```sh
$ dgraph increment --pred=counter.val.healthcheck
```

Run a read-only query for the counter predicate and does not run a mutation to increment it:

```sh
$ dgraph increment --ro
```

Run a best-effort query for the counter predicate and does not run a mutation to increment it:

```sh
$ dgraph increment --be
```

Run the increment tool 1000 times every 1 second:

```sh
$ dgraph increment --num=1000 --wait=1s
```

## Increment Tool Output

```sh
 Run increment a few times
$ dgraph increment
0410 10:31:16.379 Counter VAL: 1   [ Ts: 1 ]
$ dgraph increment
0410 10:34:53.017 Counter VAL: 2   [ Ts: 3 ]
$ dgraph increment
0410 10:34:53.648 Counter VAL: 3   [ Ts: 5 ]

 Run read-only queries to read the counter a few times
$ dgraph increment --ro
0410 10:34:57.35  Counter VAL: 3   [ Ts: 7 ]
$ dgraph increment --ro
0410 10:34:57.886 Counter VAL: 3   [ Ts: 7 ]
$ dgraph increment --ro
0410 10:34:58.129 Counter VAL: 3   [ Ts: 7 ]

 Run best-effort query to read the counter a few times
$ dgraph increment --be
0410 10:34:59.867 Counter VAL: 3   [ Ts: 7 ]
$ dgraph increment --be
0410 10:35:01.322 Counter VAL: 3   [ Ts: 7 ]
$ dgraph increment --be
0410 10:35:02.674 Counter VAL: 3   [ Ts: 7 ]

 Run a read-only query to read the counter 5 times
$ dgraph increment --ro --num=5
0410 10:35:18.812 Counter VAL: 3   [ Ts: 7 ]
0410 10:35:18.813 Counter VAL: 3   [ Ts: 7 ]
0410 10:35:18.815 Counter VAL: 3   [ Ts: 7 ]
0410 10:35:18.817 Counter VAL: 3   [ Ts: 7 ]
0410 10:35:18.818 Counter VAL: 3   [ Ts: 7 ]

 Increment the counter 5 times
$ dgraph increment --num=5
0410 10:35:24.028 Counter VAL: 4   [ Ts: 8 ]
0410 10:35:24.061 Counter VAL: 5   [ Ts: 10 ]
0410 10:35:24.104 Counter VAL: 6   [ Ts: 12 ]
0410 10:35:24.145 Counter VAL: 7   [ Ts: 14 ]
0410 10:35:24.178 Counter VAL: 8   [ Ts: 16 ]

 Increment the counter 5 times, once every second.
$ dgraph increment --num=5 --wait=1s
0410 10:35:26.95  Counter VAL: 9   [ Ts: 18 ]
0410 10:35:27.975 Counter VAL: 10   [ Ts: 20 ]
0410 10:35:28.999 Counter VAL: 11   [ Ts: 22 ]
0410 10:35:30.028 Counter VAL: 12   [ Ts: 24 ]
0410 10:35:31.054 Counter VAL: 13   [ Ts: 26 ]

 If the Alpha is too busy or unhealthy, the tool will timeout and retry.
$ dgraph increment
0410 10:36:50.857 While trying to process counter: Query error: rpc error: code = DeadlineExceeded desc = context deadline exceeded. Retrying...
```