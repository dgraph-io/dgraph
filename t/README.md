This is a Go script to run Dgraph tests. It works by creating N clusters of Dgraph, where N is
defined by the concurrency flag. It picks up packages and runs each package against one of these
N clusters. It passes the information about the endpoints via environment variables.

The script can be run like this:

```
$ go build . && ./t
# ./t --help to see the flags.
```

You can run a specific package or a specific test via this script. You can use
the concurrency flag to specify how many clusters to run.

---

This script runs many clusters of Dgraph. To make your tests work with this
script, they can get the address for any instance by passing in the name of the
instance and the port:

`testutil.ContainerAddr("alpha2", 9080)`


