+++
date = "2017-03-20T19:35:35+11:00"
title = "How To Guides"
+++

## Retrieving Debug Information

Each Dgraph data node exposes profile over `/debug/pprof` endpoint and metrics over `/debug/vars` endpoint. Each Dgraph data node has it's own profiling and metrics information. Below is a list of debugging information exposed by Dgraph and the corresponding commands to retrieve them.

### Metrics Information

If you are collecting these metrics from outside the Dgraph instance you need to pass `--expose_trace=true` flag, otherwise there metrics can be collected by connecting to the instance over localhost.

```
curl http://<IP>:<HTTP_PORT>/debug/vars
```

Metrics can also be retrieved in the Prometheus format at `/debug/prometheus_metrics`. See the [Metrics]({{< relref "deploy/index.md#metrics" >}}) section for the full list of metrics.

### Profiling Information

Profiling information is available via the `go tool pprof` profiling tool built into Go. The ["Profiling Go programs"](https://blog.golang.org/profiling-go-programs) Go blog post will help you get started with using pprof. Each Dgraph Zero and Dgraph Alpha exposes a debug endpoint at `/debug/pprof/<profile>` via the HTTP port.

```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/heap
#Fetching profile from ...
#Saved Profile in ...
```
The output of the command would show the location where the profile is stored.

In the interactive pprof shell, you can use commands like `top` to get a listing of the top functions in the profile, `web` to get a visual graph of the profile opened in a web browser, or `list` to display a code listing with profiling information overlaid.

#### CPU Profile

```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/profile
```

#### Memory Profile

```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/heap
```

#### Block Profile

Dgraph by default doesn't collect the block profile. Dgraph must be started with `--profile_mode=block` and `--block_rate=<N>` with N > 1.

```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/block
```

#### Goroutine stack

The HTTP page `/debug/pprof/` is available at the HTTP port of a Dgraph Zero or Dgraph Alpha. From this page a link to the "full goroutine stack dump" is available (e.g., on a Dgraph Alpha this page would be at `http://localhost:8080/debug/pprof/goroutine?debug=2`). Looking at the full goroutine stack can be useful to understand goroutine usage at that moment.

## Using the Debug Tool

{{% notice "note" %}}
To debug a running Dgraph cluster, first copy the postings ("p") directory to
another location. If the Dgraph cluster is not running, then you can use the
same postings directory with the debug tool.

If the “p” directory has been encrypted, then the debug tool will need to use the --keyfile <path-to-keyfile> option. This file must contain the same key that was used to encrypt the “p” directory.
{{% /notice %}}

The `dgraph debug` tool can be used to inspect Dgraph's posting list structure.
You can use the debug tool to inspect the data, schema, and indices of your
Dgraph cluster.

Some scenarios where the debug tool is useful:

- Verify that mutations committed to Dgraph have been persisted to disk.
- Verify that indices are created.
- Inspect the history of a posting list.

### Example Usage

Debug the p directory.

```sh
$ dgraph debug --postings ./p
```

Debug the p directory, not opening in read-only mode. This is typically necessary when the database was not closed properly.

```sh
$ dgraph debug --postings ./p --readonly=false
```

Debug the p directory, only outputing the keys for the predicate `name`.

```sh
$ dgraph debug --postings ./p --readonly=false --pred=name
```

Debug the p directory, looking up a particular key:

```sh
$ dgraph debug --postings ./p --lookup 00000b6465736372697074696f6e020866617374
```

Debug the p directory, inspecting the history of a particular key:

```sh
$ dgraph debug --postings ./p --lookup 00000b6465736372697074696f6e020866617374 --history
```

Debug an encrypted p directory with the key in a local file at the path  ./key_file:

```sh
$ dgraph debug --postings ./p --keyfile ./key_file
```


{{% notice "note" %}}
The key file contains the key used to decrypt/encrypt the db. This key should be kept secret. As a best practice, 

- Do not store the key file on the disk permanently. Back it up in a safe place and delete it after using it with the debug tool.

- If the above is not possible, make sure correct privileges are set on the keyfile. Only the user who owns the dgraph process should be able to read / write the key file: `chmod 600` 
{{% /notice %}}

### Debug Tool Output

Let's go over an example with a Dgraph cluster with the following schema with a term index, full-text index, and two separately committed mutations:

```sh
$ curl localhost:8080/alter -d '
  name: string @index(term) .
  url: string .
  description: string @index(fulltext) .
'
```

```sh
$ curl -H "Content-Type: application/rdf" localhost:8080/mutate?commitNow=true -d '{
  set {
    _:dgraph <name> "Dgraph" .
    _:dgraph <dgraph.type> "Software" .
    _:dgraph <url> "https://github.com/dgraph-io/dgraph" .
    _:dgraph <description> "Fast, Transactional, Distributed Graph Database." .
  }
}'
```

```sh
$ curl -H "Content-Type: application/rdf" localhost:8080/mutate?commitNow=true -d '{
  set {
    _:badger <name> "Badger" .
    _:badger <dgraph.type> "Software" .
    _:badger <url> "https://github.com/dgraph-io/badger" .
    _:badger <description> "Embeddable, persistent and fast key-value (KV) database written in pure Go." .
  }
}'
```

After stopping Dgraph, you can run the debug tool to inspect the postings directory:

{{% notice "note" %}}
The debug output can be very large. Typically you would redirect the debug tool to a file first for easier analysis.
{{% /notice %}}

```sh
$ dgraph debug --postings ./p
```

```text
Opening DB: ./p
Min commit: 1. Max commit: 5, w.r.t 18446744073709551615
prefix =
{d} {v.ok} attr: url uid: 1  key: 00000375726c000000000000000001 item: [71, b0100] ts: 3
{d} {v.ok} attr: url uid: 2  key: 00000375726c000000000000000002 item: [71, b0100] ts: 5
{d} {v.ok} attr: name uid: 1  key: 0000046e616d65000000000000000001 item: [43, b0100] ts: 3
{d} {v.ok} attr: name uid: 2  key: 0000046e616d65000000000000000002 item: [43, b0100] ts: 5
{i} {v.ok} attr: name term: [1] badger  key: 0000046e616d650201626164676572 item: [30, b0100] ts: 5
{i} {v.ok} attr: name term: [1] dgraph  key: 0000046e616d650201646772617068 item: [30, b0100] ts: 3
{d} {v.ok} attr: _predicate_ uid: 1  key: 00000b5f7072656469636174655f000000000000000001 item: [104, b0100] ts: 3
{d} {v.ok} attr: _predicate_ uid: 2  key: 00000b5f7072656469636174655f000000000000000002 item: [104, b0100] ts: 5
{d} {v.ok} attr: description uid: 1  key: 00000b6465736372697074696f6e000000000000000001 item: [92, b0100] ts: 3
{d} {v.ok} attr: description uid: 2  key: 00000b6465736372697074696f6e000000000000000002 item: [119, b0100] ts: 5
{i} {v.ok} attr: description term: [8] databas  key: 00000b6465736372697074696f6e020864617461626173 item: [38, b0100] ts: 5
{i} {v.ok} attr: description term: [8] distribut  key: 00000b6465736372697074696f6e0208646973747269627574 item: [40, b0100] ts: 3
{i} {v.ok} attr: description term: [8] embedd  key: 00000b6465736372697074696f6e0208656d62656464 item: [37, b0100] ts: 5
{i} {v.ok} attr: description term: [8] fast  key: 00000b6465736372697074696f6e020866617374 item: [35, b0100] ts: 5
{i} {v.ok} attr: description term: [8] go  key: 00000b6465736372697074696f6e0208676f item: [33, b0100] ts: 5
{i} {v.ok} attr: description term: [8] graph  key: 00000b6465736372697074696f6e02086772617068 item: [36, b0100] ts: 3
{i} {v.ok} attr: description term: [8] kei  key: 00000b6465736372697074696f6e02086b6569 item: [34, b0100] ts: 5
{i} {v.ok} attr: description term: [8] kv  key: 00000b6465736372697074696f6e02086b76 item: [33, b0100] ts: 5
{i} {v.ok} attr: description term: [8] persist  key: 00000b6465736372697074696f6e020870657273697374 item: [38, b0100] ts: 5
{i} {v.ok} attr: description term: [8] pure  key: 00000b6465736372697074696f6e020870757265 item: [35, b0100] ts: 5
{i} {v.ok} attr: description term: [8] transact  key: 00000b6465736372697074696f6e02087472616e73616374 item: [39, b0100] ts: 3
{i} {v.ok} attr: description term: [8] valu  key: 00000b6465736372697074696f6e020876616c75 item: [35, b0100] ts: 5
{i} {v.ok} attr: description term: [8] written  key: 00000b6465736372697074696f6e02087772697474656e item: [38, b0100] ts: 5
{s} {v.ok} attr: url key: 01000375726c item: [13, b0001] ts: 1
{s} {v.ok} attr: name key: 0100046e616d65 item: [23, b0001] ts: 1
{s} {v.ok} attr: _predicate_ key: 01000b5f7072656469636174655f item: [31, b0001] ts: 1
{s} {v.ok} attr: description key: 01000b6465736372697074696f6e item: [41, b0001] ts: 1
{s} {v.ok} attr: dgraph.type key: 01000b6467726170682e74797065 item: [40, b0001] ts: 1
Found 28 keys
```

Each line in the debug output contains a prefix indicating the type of the key: `{d}`: Data key; `{i}`: Index key; `{c}`: Count key; `{r}`: Reverse key; `{s}`: Schema key. In the debug output above, we see data keys, index keys, and schema keys.

Each index key has a corresponding index type. For example, in `attr: name term: [1] dgraph` the `[1]` shows that this is the term index ([0x1][tok_term]); in `attr: description term: [8] fast`, the `[8]` shows that this is the full-text index ([0x8][tok_fulltext]). These IDs match the index IDs in [tok.go][tok].

[tok_term]: https://github.com/dgraph-io/dgraph/blob/ce82aaafba3d9e57cf5ea1aeb9b637193441e1e2/tok/tok.go#L39
[tok_fulltext]: https://github.com/dgraph-io/dgraph/blob/ce82aaafba3d9e57cf5ea1aeb9b637193441e1e2/tok/tok.go#L48
[tok]: https://github.com/dgraph-io/dgraph/blob/ce82aaafba3d9e57cf5ea1aeb9b637193441e1e2/tok/tok.go#L37-L53

### Key Lookup

Every key can be inspected further with the `--lookup` flag for the specific key.

```sh
$ dgraph debug --postings ./p --lookup 00000b6465736372697074696f6e020866617374
```

```text
Opening DB: ./p
Min commit: 1. Max commit: 5, w.r.t 18446744073709551615
 Key: 00000b6465736372697074696f6e0208676f Length: 2
 Uid: 1 Op: 1
 Uid: 2 Op: 1
```

For data keys, a lookup shows its type and value. Below, we see that the key for `attr: url uid: 1` is a string value.

```sh
$ dgraph debug --postings ./p --lookup 00000375726c000000000000000001
```

```text
Opening DB: ./p
Min commit: 1. Max commit: 5, w.r.t 18446744073709551615
 Key: 0000046e616d65000000000000000001 Length: 1
 Uid: 18446744073709551615 Op: 1  Type: STRING.  String Value: "https://github.com/dgraph-io/dgraph"
```

For index keys, a lookup shows the UIDs that are part of this index. Below, we see that the `fast` index for the `<description>` predicate has UIDs 0x1 and 0x2.

```sh
$ dgraph debug --postings ./p --lookup 00000b6465736372697074696f6e020866617374
```

```text
Opening DB: ./p
Min commit: 1. Max commit: 5, w.r.t 18446744073709551615
 Key: 00000b6465736372697074696f6e0208676f Length: 2
 Uid: 1 Op: 1
 Uid: 2 Op: 1
```

### Key History

You can also look up the history of values for a key using the `--history` option.

```sh
$ dgraph debug --postings ./p --lookup 00000b6465736372697074696f6e020866617374 --history
```
```text
Opening DB: ./p
Min commit: 1. Max commit: 5, w.r.t 18446744073709551615
==> key: 00000b6465736372697074696f6e020866617374. PK: &{byteType:2 Attr:description Uid:0 Termfast Count:0 bytePrefix:0}
ts: 5 {item}{delta}
 Uid: 2 Op: 1

ts: 3 {item}{delta}
 Uid: 1 Op: 1
```

Above, we see that UID 0x1 was committed to this index at ts 3, and UID 0x2 was committed to this index at ts 5.

The debug output also shows UserMeta information:

- `{complete}`: Complete posting list
- `{uid}`: UID posting list
- `{delta}`: Delta posting list
- `{empty}`: Empty posting list
- `{item}`: Item posting list
- `{deleted}`: Delete marker

## Using the Dgraph Sentry Integration

Sentry is a powerful service that allows applications to send arbitrary events, messages, exceptions, bread-crumbs (logs) to your sentry account. In simplest terms, it is a dial-home service but also has a  rich feature set including event filtering, data scrubbing, several SDKs, custom and release tagging, as well as integration with 3rd party tools such as Slack, GitHub.

Although Sentry reporting is on by default, starting from v20.03.1 and v20.07.0, there is a configuration flag `enable-sentry` which can be used to completely turn off Sentry events reporting. 

### Basic Integration

**Panics (runtime and manual)**

* As of now, at Dgraph, we use Sentry reporting for capturing panics only. For manual panics anywhere in the code, sentry.CaptureException() API is called. 

* For runtime panics, Sentry does not have any native method. After further research, we chose the approach of a wrapper process to capture these panics. The basic idea for this is that whenever a dgraph instance is started, a 2nd monitoring process is started whose only job is to monitor the stderr for panics of the monitored process. When a panic is seen, it is reported back to sentry via the CaptureException API. 

**Reporting**

Each event is tagged with the release version, environment, timestamp, tags and the panic backtrace as explained below.
**Release:**

  - This is the release version string of the Dgraph instance.
  
**Environments:**

We have defined 4 environments:

**dev-oss / dev-enterprise**: These are events seen on non-released / local developer builds.

**prod-oss/prod-enterprise**: These are events on released version such as v20.03.0. Events in this category are also sent on a slack channel private to Dgraph

**Tags:**

Tags are key-value pairs that provide additional context for an event. We have defined the following tags:

`dgraph`: This tag can have values “zero” or “alpha” depending on which sub-command saw the panic/exception.

### Data Handling

We strive to handle your data with care in a variety of ways when sending events to Sentry

1. **Event Selection:** As of now, only panic events are sent to Sentry from Dgraph. 
2. **Data in Transit:** Events sent from the SDK to the Sentry server are encrypted on the wire with industry-standard TLS protocol with 256 bit AES Cipher.
3. **Data at rest:** Events on the Sentry server are also encrypted with 256 bit AES cipher. Sentry is hosted on GCP and as such physical access is tightly controlled. Logical access is only available to sentry approved officials.
4. **Data Retention:** Sentry stores events only for 90 days after which they are removed permanently.
5. **Data Scrubbing**: The Data Scrcubber option (default: on) in Sentry’s settings ensures PII doesn’t get sent to or stored on Sentry’s servers, automatically removing any values that look like they contain sensitive information for values that contain various strings. The strings we currently monitor and scrub are:

- `password`
- `secret`
- `passwd`
- `api_key`
- `apikey`
- `access_token`
- `auth_token`
- `credentials`
- `mysql_pwd`
- `stripetoken`
- `card[number]`
- `ip addresses` 

## Using the Increment Tool

The `dgraph increment` tool increments a counter value transactionally. The
increment tool can be used as a health check that an Alpha is able to service
transactions for both queries and mutations.

### Example Usage

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

### Increment Tool Output

```sh
# Run increment a few times
$ dgraph increment
0410 10:31:16.379 Counter VAL: 1   [ Ts: 1 ]
$ dgraph increment
0410 10:34:53.017 Counter VAL: 2   [ Ts: 3 ]
$ dgraph increment
0410 10:34:53.648 Counter VAL: 3   [ Ts: 5 ]

# Run read-only queries to read the counter a few times
$ dgraph increment --ro
0410 10:34:57.35  Counter VAL: 3   [ Ts: 7 ]
$ dgraph increment --ro
0410 10:34:57.886 Counter VAL: 3   [ Ts: 7 ]
$ dgraph increment --ro
0410 10:34:58.129 Counter VAL: 3   [ Ts: 7 ]

# Run best-effort query to read the counter a few times
$ dgraph increment --be
0410 10:34:59.867 Counter VAL: 3   [ Ts: 7 ]
$ dgraph increment --be
0410 10:35:01.322 Counter VAL: 3   [ Ts: 7 ]
$ dgraph increment --be
0410 10:35:02.674 Counter VAL: 3   [ Ts: 7 ]

# Run a read-only query to read the counter 5 times
$ dgraph increment --ro --num=5
0410 10:35:18.812 Counter VAL: 3   [ Ts: 7 ]
0410 10:35:18.813 Counter VAL: 3   [ Ts: 7 ]
0410 10:35:18.815 Counter VAL: 3   [ Ts: 7 ]
0410 10:35:18.817 Counter VAL: 3   [ Ts: 7 ]
0410 10:35:18.818 Counter VAL: 3   [ Ts: 7 ]

# Increment the counter 5 times
$ dgraph increment --num=5
0410 10:35:24.028 Counter VAL: 4   [ Ts: 8 ]
0410 10:35:24.061 Counter VAL: 5   [ Ts: 10 ]
0410 10:35:24.104 Counter VAL: 6   [ Ts: 12 ]
0410 10:35:24.145 Counter VAL: 7   [ Ts: 14 ]
0410 10:35:24.178 Counter VAL: 8   [ Ts: 16 ]

# Increment the counter 5 times, once every second.
$ dgraph increment --num=5 --wait=1s
0410 10:35:26.95  Counter VAL: 9   [ Ts: 18 ]
0410 10:35:27.975 Counter VAL: 10   [ Ts: 20 ]
0410 10:35:28.999 Counter VAL: 11   [ Ts: 22 ]
0410 10:35:30.028 Counter VAL: 12   [ Ts: 24 ]
0410 10:35:31.054 Counter VAL: 13   [ Ts: 26 ]

# If the Alpha is too busy or unhealthy, the tool will timeout and retry.
$ dgraph increment
0410 10:36:50.857 While trying to process counter: Query error: rpc error: code = DeadlineExceeded desc = context deadline exceeded. Retrying...
```

## Giving Nodes a Type

It's often useful to give the nodes in a graph *types* (also commonly referred
to as *labels* or *kinds*). You can do so using the [type system]({{< relref "query-language/index.md#type-system" >}}).

## Loading CSV Data

[Dgraph mutations]({{< relref "mutations/index.md" >}}) are accepted in RDF
N-Quad and JSON formats. To load CSV-formatted data into Dgraph, first convert
the dataset into one of the accepted formats and then load the resulting dataset
into Dgraph. This section demonstrates converting CSV into JSON. There are
many tools available to convert CSV to JSON. For example, you can use
[`d3-dsv`](https://github.com/d3/d3-dsv)'s `csv2json` tool as shown below:

```csv
Name,URL
Dgraph,https://github.com/dgraph-io/dgraph
Badger,https://github.com/dgraph-io/badger
```

```sh
$ csv2json names.csv --out names.json
$ cat names.json | jq '.'
[
  {
    "Name": "Dgraph",
    "URL": "https://github.com/dgraph-io/dgraph"
  },
  {
    "Name": "Badger",
    "URL": "https://github.com/dgraph-io/badger"
  }
]
```

This JSON can be loaded into Dgraph via the programmatic clients. This follows
the [JSON Mutation Format]({{< relref "mutations#json-mutation-format" >}}).
Note that each JSON object in the list above will be assigned a unique UID since
the `uid` field is omitted.

[The Ratel UI (and HTTP clients) expect JSON data to be stored within the `"set"`
key]({{< relref "mutations/index.md#json-syntax-using-raw-http-or-ratel-ui"
>}}). You can use `jq` to transform the JSON into the correct format:

```sh
$ cat names.json | jq '{ set: . }'
```
```json
{
  "set": [
    {
      "Name": "Dgraph",
      "URL": "https://github.com/dgraph-io/dgraph"
    },
    {
      "Name": "Badger",
      "URL": "https://github.com/dgraph-io/badger"
    }
  ]
}
```

Let's say you have CSV data in a file named connects.csv that's connecting nodes
together. Here, the `connects` field should `uid` type.

```csv
uid,connects
_:a,_:b
_:a,_:c
_:c,_:d
_:d,_:a
```

{{% notice "note" %}}
To reuse existing integer IDs from a CSV file as UIDs in Dgraph, use Dgraph Zero's [assign endpoint]({{< relref "deploy/index.md#more-about-dgraph-zero" >}}) before data loading to allocate a range of UIDs that can be safely assigned.
{{% /notice %}}

To get the correct JSON format, you can convert the CSV into JSON and use `jq`
to transform it in the correct format where the `connects` edge is a node uid:

```sh
$ csv2json connects.csv | jq '[ .[] | { uid: .uid, connects: { uid: .connects } } ]'
```

```json
[
  {
    "uid": "_:a",
    "connects": {
      "uid": "_:b"
    }
  },
  {
    "uid": "_:a",
    "connects": {
      "uid": "_:c"
    }
  },
  {
    "uid": "_:c",
    "connects": {
      "uid": "_:d"
    }
  },
  {
    "uid": "_:d",
    "connects": {
      "uid": "_:a"
    }
  }
]
```

You can modify the `jq` transformation to output the mutation format accepted by
Ratel UI and HTTP clients:

```sh
$ csv2json connects.csv | jq '{ set: [ .[] | {uid: .uid, connects: { uid: .connects } } ] }'
```
```json
{
  "set": [
    {
      "uid": "_:a",
      "connects": {
        "uid": "_:b"
      }
    },
    {
      "uid": "_:a",
      "connects": {
        "uid": "_:c"
      }
    },
    {
      "uid": "_:c",
      "connects": {
        "uid": "_:d"
      }
    },
    {
      "uid": "_:d",
      "connects": {
        "uid": "_:a"
      }
    }
  ]
}
```

## A Simple Login System

{{% notice "note" %}}
This example is based on part of the [transactions in
v0.9](https://blog.dgraph.io/post/v0.9/) blogpost. Error checking has been
omitted for brevity.
{{% /notice %}}

Schema is assumed to be:
```
// @upsert directive is important to detect conflicts.
email: string @index(exact) @upsert . # @index(hash) would also work
pass: password .
```

```
// Create a new transaction. The deferred call to Discard
// ensures that server-side resources are cleaned up.
txn := client.NewTxn()
defer txn.Discard(ctx)

// Create and execute a query to looks up an email and checks if the password
// matches.
q := fmt.Sprintf(`
    {
        login_attempt(func: eq(email, %q)) {
            checkpwd(pass, %q)
        }
    }
`, email, pass)
resp, err := txn.Query(ctx, q)

// Unmarshal the response into a struct. It will be empty if the email couldn't
// be found. Otherwise it will contain a bool to indicate if the password matched.
var login struct {
    Account []struct {
        Pass []struct {
            CheckPwd bool `json:"checkpwd"`
        } `json:"pass"`
    } `json:"login_attempt"`
}
err = json.Unmarshal(resp.GetJson(), &login); err != nil {

// Now perform the upsert logic.
if len(login.Account) == 0 {
    fmt.Println("Account doesn't exist! Creating new account.")
    mu := &protos.Mutation{
        SetJson: []byte(fmt.Sprintf(`{ "email": %q, "pass": %q }`, email, pass)),
    }
    _, err = txn.Mutate(ctx, mu)
    // Commit the mutation, making it visible outside of the transaction.
    err = txn.Commit(ctx)
} else if login.Account[0].Pass[0].CheckPwd {
    fmt.Println("Login successful!")
} else {
    fmt.Println("Wrong email or password.")
}
```
## Load balancing queries with Nginx

There might be times when you'll want to set up a load balancer to accomplish goals such as increasing the utilization of your database by sending queries from the app to multiple database server replicas. You can follow these steps to get started with that.

### Download ZIP

Download the contents of this gist's ZIP file and extract it to a directory called `graph-nginx`

```
mkdir dgraph-nginx
cd dgraph-nginx
wget -O dgraph-nginx.zip https://gist.github.com/danielmai/0cf7647b27c7626ad8944c4245a9981e/archive/5a2f1a49ca2f77bc39981749e4783e3443eb3ad9.zip
unzip -j dgraph-nginx.zip
```
Two files will be created: `docker-compose.yml` and `nginx.conf`.

### Start Dgraph cluster

Start a 6-node Dgraph cluster (3 Dgraph Zero, 3 Dgraph Alpha, replication setting 3) by starting the Docker Compose config:

```
docker-compose up
```
### Use the increment tool to start a gRPC LB

In a different shell, run the `dgraph increment` [docs](https://dgraph.io/docs/howto/#using-the-increment-tool) tool against the Nginx gRPC load balancer (`nginx:9080`):

```
docker-compose exec alpha1 dgraph increment --alpha nginx:9080 --num=10
```
If you have `dgraph` installed on your host machine, then you can also run this from the host:

```
dgraph increment --alpha localhost:9080 --num=10
```
The increment tool uses the Dgraph Go client to establish a gRPC connection against the `--alpha` flag and transactionally increments a counter predicate `--num` times.

### Check logs

In the Nginx access logs (in the docker-compose up shell window), you'll see access logs like the following:

{{% notice "note" %}}
It is important to take into account with gRPC load balancing that every request hits a different Alpha, potentially increasing read throughput.
{{% /notice %}}

```
nginx_1   | [15/Jan/2020:03:12:02 +0000] 172.20.0.9 - - -  nginx to: 172.20.0.7:9080: POST /api.Dgraph/Query HTTP/2.0 200 upstream_response_time 0.008 msec 1579057922.135 request_time 0.009
nginx_1   | [15/Jan/2020:03:12:02 +0000] 172.20.0.9 - - -  nginx to: 172.20.0.2:9080: POST /api.Dgraph/Query HTTP/2.0 200 upstream_response_time 0.012 msec 1579057922.149 request_time 0.013
nginx_1   | [15/Jan/2020:03:12:02 +0000] 172.20.0.9 - - -  nginx to: 172.20.0.5:9080: POST /api.Dgraph/Query HTTP/2.0 200 upstream_response_time 0.008 msec 1579057922.162 request_time 0.012
nginx_1   | [15/Jan/2020:03:12:02 +0000] 172.20.0.9 - - -  nginx to: 172.20.0.7:9080: POST /api.Dgraph/Query HTTP/2.0 200 upstream_response_time 0.012 msec 1579057922.176 request_time 0.013
nginx_1   | [15/Jan/2020:03:12:02 +0000] 172.20.0.9 - - -  nginx to: 172.20.0.2:9080: POST /api.Dgraph/Query HTTP/2.0 200 upstream_response_time 0.012 msec 1579057922.188 request_time 0.011
nginx_1   | [15/Jan/2020:03:12:02 +0000] 172.20.0.9 - - -  nginx to: 172.20.0.5:9080: POST /api.Dgraph/Query HTTP/2.0 200 upstream_response_time 0.016 msec 1579057922.202 request_time 0.013
```
These logs show that traffic os being load balanced to the following upstream addresses defined in alpha_grpc in nginx.conf:

- `nginx to: 172.20.0.7`
- `nginx to: 172.20.0.2`
- `nginx to: 172.20.0.5`

By default, Nginx load balancing is done round-robin.

## Upserts

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

### Upsert Procedure

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

### Upsert Block

You can also use the `Upsert Block` to achieve the upsert procedure in a single
 mutation. The request will contain both the query and the mutation as explained
[here]({{< relref "mutations/index.md#upsert-block" >}}).

### Conflicts

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

## Run Jepsen tests

1. Clone the jepsen repo at [https://github.com/jepsen-io/jepsen](https://github.com/jepsen-io/jepsen).

```sh
git clone git@github.com:jepsen-io/jepsen.git
```

2. Run the following command to setup the instances from the repo.

```sh
cd docker && ./up.sh
```

This should start 5 jepsen nodes in docker containers.

3. Now ssh into `jepsen-control` container and run the tests.

{{% notice "note" %}}
You can use the [transfer](https://github.com/dgraph-io/dgraph/blob/master/contrib/nightly/transfer.sh) script to build the Dgraph binary and upload the tarball to https://transfer.sh, which gives you a url that can then be used in the Jepsen tests (using --package-url flag).
{{% /notice %}}



```sh
docker exec -it jepsen-control bash
```

```sh
root@control:/jepsen# cd dgraph
root@control:/jepsen/dgraph# lein run test -w upsert

# Specify a --package-url

root@control:/jepsen/dgraph# lein run test --force-download --package-url https://github.com/dgraph-io/dgraph/releases/download/nightly/dgraph-linux-amd64.tar.gz -w upsert
```

## Migrate to Dgraph v1.1

### Schema types: scalar `uid` and list `[uid]`

The semantics of predicates of type `uid` has changed in Dgraph 1.1. Whereas before all `uid` predicates implied a one-to-many relationship, now a one-to-one relationship or a one-to-many relationship can be expressed.

```
friend: [uid] .
best_friend: uid .
```

In the above, the predicate `friend` allows a one-to-many relationship (i.e a person can have more than one friend) and the predicate best_friend can be at most a one-to-one relationship.

This syntactic meaning is consistent with the other types, e.g., `string` indicating a single-value string and `[string]` representing many strings. This change makes the `uid` type work similarly to other types.

To migrate existing schemas from Dgraph v1.0 to Dgraph v1.1, update the schema file from an export so all predicates of type `uid` are changed to `[uid]`. Then use the updated schema when loading data into Dgraph v1.1. For example, for the following schema:

```text
name: string .
friend: uid .
```

becomes

```text
name: string .
friend: [uid] .
```
### Type system

The new [type system]({{< relref "query-language/index.md#type-system" >}}) introduced in Dgraph 1.1 should not affect migrating data from a previous version. However, a couple of features in the query language will not work as they did before: `expand()` and `_predicate_`.

The reason is that the internal predicate that associated each node with its predicates (called `_predicate_`) has been removed. Instead, to get the predicates that belong to a node, the type system is used.

#### `expand()`

Expand queries will not work until the type system has been properly set up. For example, the following query will return an empty result in Dgraph 1.1 if the node 0xff has no type information.

```text
{
  me(func: uid(0xff)) {
    expand(_all_)
  }
}
```

To make it work again, add a type definition via the alter endpoint. Let’s assume the node in the previous example represents a person. Then, the basic Person type could be defined as follows:

```text
type Person {
  name
  age
}
```

After that, the node is associated with the type by adding the following RDF triple to Dgraph (using a mutation):

```text
<0xff> <dgraph.type> "Person" .
```

After that, the results of the query in both Dgraph v1.0 and Dgraph v1.1 should be the same.

#### `_predicate_`

The other consequence of removing `_predicate_` is that it cannot be referenced explicitly in queries. In Dgraph 1.0, the following query returns the predicates of the node 0xff.

```ql
{
  me(func: uid(0xff)) {
     _predicate_ # NOT available in Dgraph v1.1
  }
}
```

**There’s no exact equivalent of this behavior in Dgraph 1.1**, but the information can be queried by first querying for the types associated with that node with the query

```text
{
  me(func: uid(0xff)) {
     dgraph.type
  }
}
```

And then retrieving the definition of each type in the results using a schema query.

```text
schema(type: Person) {}
```

### Live Loader and Bulk Loader command-line flags

#### File input flags
In Dgraph v1.1, both the Dgraph Live Loader and Dgraph Bulk Loader tools support loading data in either RDF format or JSON format. To simplify the command-line interface for these tools, the `-r`/`--rdfs` flag has been removed in favor of `-f/--files`. The new flag accepts file or directory paths for either data format. By default, the tools will infer the file type based on the file suffix, e.g., `.rdf` and `.rdf.gz` or `.json` and `.json.gz` for RDF data or JSON data, respectively. To ignore the filenames and set the format explicitly, the `--format` flag can be set to `rdf` or `json`.

Before (in Dgraph v1.0):

```sh
dgraph live -r data.rdf.gz
```

Now (in Dgraph v1.1):

```sh
dgraph live -f data.rdf.gz
```

#### Dgraph Alpha address flag
For Dgraph Live Loader, the flag to specify the Dgraph Alpha address  (default: `127.0.0.1:9080`) has changed from `-d`/`--dgraph` to `-a`/`--alpha`.

Before (in Dgraph v1.0):

```sh
dgraph live -d 127.0.0.1:9080
```

Now (in Dgraph v1.1):

```sh
dgraph live -a 127.0.0.1:9080
```
### HTTP API

For HTTP API users (e.g., Curl, Postman), the custom Dgraph headers have been removed in favor of standard HTTP headers and query parameters.

#### Queries

There are two accepted `Content-Type` headers for queries over HTTP: `application/graphql+-` or `application/json`.

A `Content-Type` must be set to run a query.

Before (in Dgraph v1.0):

```sh
curl localhost:8080/query -d '{
  q(func: eq(name, "Dgraph")) {
    name
  }
}'
```

Now (in Dgraph v1.1):

```sh
curl -H 'Content-Type: application/graphql+-' localhost:8080/query -d '{
  q(func: eq(name, "Dgraph")) {
    name
  }
}'
```

For queries using [GraphQL Variables]({{< relref "query-language/index.md#graphql-variables" >}}), the query must be sent via the `application/json` content type, with the query and variables sent in a JSON payload:

Before (in Dgraph v1.0):

```sh
curl -H 'X-Dgraph-Vars: {"$name": "Alice"}' localhost:8080/query -d 'query qWithVars($name: string) {
  q(func: eq(name, $name)) {
    name
  }
}
```

Now (in Dgraph v1.1):

```sh
curl -H 'Content-Type: application/json' localhost:8080/query -d '{
  "query": "query qWithVars($name: string) { q(func: eq(name, $name)) { name } }",
  "variables": {"$name": "Alice"}
}'
```

#### Mutations

There are two accepted Content-Type headers for mutations over HTTP: `Content-Type: application/rdf` or `Content-Type: application/json`.

A `Content-Type` must be set to run a mutation.

These Content-Type headers supercede the Dgraph v1.0.x custom header `X-Dgraph-MutationType` to set the mutation type as RDF or JSON.

To commit the mutation immediately, use the query parameter `commitNow=true`. This replaces the custom header `X-Dgraph-CommitNow: true` from Dgraph v1.0.x.

Before (in Dgraph v1.0)

```sh
curl -H 'X-Dgraph-CommitNow: true' localhost:8080/mutate -d '{
  set {
    _:n <name> "Alice" .
    _:n <dgraph.type> "Person" .
  }
}'
```

Now (in Dgraph v1.1):

```sh
curl -H 'Content-Type: application/rdf' localhost:8080/mutate?commitNow=true -d '{
  set {
    _:n <name> "Alice" .
    _:n <dgraph.type> "Person" .
  }
}'
```

For JSON mutations, set the `Content-Type` header to `application/json`.

Before (in Dgraph v1.0):

```sh
curl -H 'X-Dgraph-MutationType: json' -H "X-Dgraph-CommitNow: true" locahost:8080/mutate -d '{
  "set": [
    {
      "name": "Alice"
    }
  ]
}'
```

Now (in Dgraph v1.1):

```sh
curl -H 'Content-Type: application/json' locahost:8080/mutate?commitNow=true -d '{
  "set": [
    {
      "name": "Alice"
    }
  ]
}'
```
