+++
date = "2017-03-20T22:25:17+11:00"
title = "Using the Debug Tool"
weight = 2
[menu.main]
    parent = "howto"
+++

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

## Example Usage

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

## Debug Tool Output

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

## Key Lookup

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

## Key History

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