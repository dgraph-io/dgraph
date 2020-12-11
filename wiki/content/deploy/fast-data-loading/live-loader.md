+++
date = "2017-03-20T22:25:17+11:00"
title = "Live Loader"
weight = 12
[menu.main]
    parent = "fast-data-loading"
+++

Dgraph Live Loader (run with `dgraph live`) is a small helper program which reads RDF N-Quads from a gzipped file, batches them up, creates mutations (using the go client) and shoots off to Dgraph.

Dgraph Live Loader correctly handles assigning unique IDs to blank nodes across multiple files, and can optionally persist them to disk to save memory, in case the loader was re-run.

{{% notice "note" %}} Dgraph Live Loader can optionally write the `xid`->`uid` mapping to a directory specified using the `--xidmap` flag, which can reused
given that live loader completed successfully in the previous run.{{% /notice %}}

{{% notice "note" %}} Live Loader only accept [RDF N-Quad/Triple
data](https://www.w3.org/TR/n-quads/) or JSON in plain or gzipped format. Data
in other formats must be converted.{{% /notice %}}

```sh
$ dgraph live --help # To see the available flags.

# Read RDFs or JSON from the passed file, and send them to Dgraph on localhost:9080.
$ dgraph live -f <path-to-gzipped-RDF-or-JSON-file>

# Read multiple RDFs or JSON from the passed path, and send them to Dgraph on localhost:9080.
$ dgraph live -f <./path-to-gzipped-RDF-or-JSON-files>

# Read multiple files strictly by name.
$ dgraph live -f <file1.rdf, file2.rdf>

# Use compressed gRPC connections to and from Dgraph.
$ dgraph live -C -f <path-to-gzipped-RDF-or-JSON-file>

# Read RDFs and a schema file and send to Dgraph running at given address.
$ dgraph live -f <path-to-gzipped-RDf-or-JSON-file> -s <path-to-schema-file> -a <dgraph-alpha-address:grpc_port> -z <dgraph-zero-address:grpc_port>
```

### Encrypted imports via Live Loader (Enterprise Feature)

A new flag `--encryption_key_file` is added to the Live Loader. This option is required to decrypt the encrypted export data and schema files. Once the export files are decrypted, the Live Loader streams the data to a live Alpha instance.
Alternatively, starting with v20.07.0, the `vault_*` options can be used to decrypt the encrypted export and schema files.

{{% notice "note" %}}
If the live Alpha instance has encryption turned on, the `p` directory will be encrypted. Otherwise, the `p` directory is unencrypted.
{{% /notice %}}

#### Encrypted RDF/JSON file and schema via Live Loader

```sh
dgraph live -f <path-to-encrypted-gzipped-RDF-or-JSON-file> -s <path-to-encrypted-schema> --encryption_keyfile <path-to-keyfile-to-decrypt-files>
```

### Batch Upserts in Live Loader

With batch upserts in Live Loader, you can insert big data-sets (multiple files) into an existing cluster that might contain nodes that already exist in the graph.
Live Loader generates an `upsertPredicate` query for each of the ids found in the request, while
adding the corresponding `xid` to that `uid`. The added mutation is only useful if the `uid` doesn't exists.

The `-U, --upsertPredicate` flag runs the Live Loader in upsertPredicate mode. The provided predicate needs to be indexed, and the Loader will use it to store blank nodes as a `xid`.

{{% notice "note" %}}
When the `upsertPredicate` already exists in the data, the existing node with this `xid` is modified and no new node is added.
{{% /notice %}}

For example:
```sh
dgraph live -f <path-to-gzipped-RDf-or-JSON-file> -s <path-to-schema-file> -U <xid>
```

### Other Live Loader options

`--new_uids` (default: `false`): Assign new UIDs instead of using the existing
UIDs in data files. This is useful to avoid overriding the data in a DB already
in operation.

`-f, --files`: Location of *.rdf(.gz) or *.json(.gz) file(s) to load. It can
load multiple files in a given path. If the path is a directory, then all files
ending in .rdf, .rdf.gz, .json, and .json.gz will be loaded.

`--format`: Specify file format (`rdf` or `json`) instead of getting it from
filenames. This is useful if you need to define a strict format manually.

`-b, --batch` (default: `1000`): Number of N-Quads to send as part of a mutation.

`-c, --conc` (default: `10`): Number of concurrent requests to make to Dgraph.
Do not confuse with `-C`.

`-C, --use_compression` (default: `false`): Enable compression for connections to and from the
Alpha server.

`-a, --alpha` (default: `localhost:9080`): Dgraph Alpha gRPC server address to connect for live loading. This can be a comma-separated list of Alphas addresses in the same cluster to distribute the load, e.g.,  `"alpha:grpc_port,alpha2:grpc_port,alpha3:grpc_port"`.

`-x, --xidmap` (default: disabled. Need a path): Store `xid` to `uid` mapping to a directory. Dgraph will save all identifiers used in the load for later use in other data ingest operations. The mapping will be saved in the path you provide and you must indicate that same path in the next load. 

{{% notice "tip" %}}
Using the `--xidmap` flag is recommended if you have full control over your identifiers (Blank-nodes). Because the identifier will be mapped to a specific `uid`.
{{% /notice %}}

`--ludicrous_mode` (default: `false`): This option allows the user to notify Live Loader that the Alpha server is running in ludicrous mode.
Live Loader, by default, does smart batching of data to avoid transaction conflicts, which improves the performance in normal mode.
Since there's no conflict detection in ludicrous mode, smart batching is disabled to speed up the data ingestion further.

{{% notice "note" %}}
The `--ludicrous_mode` option should only be used if Dgraph is also running in [ludicrous mode]({{< relref "ludicrous-mode.md" >}}).
{{% /notice %}}

`-U, --upsertPredicate` (default: disabled): Runs Live Loader in `upsertPredicate` mode. The provided value will be used to store blank nodes as a `xid`.

`--vault_*` flags specifies the Vault server address, role id, secret id and
field that contains the encryption key that can be used to decrypt the encrypted export.

### `upsertPredicate` Example

You might find that discrete pieces of information regarding entities are arriving through independent data feeds. 
The feeds might involve adding basic information (first and last name), income, and address in separate files. 
You can use the live loader to correlate individual records from these files and combine attributes to create a consolidated Dgraph node. 

Start by adding the following schema:

```
<address>: [uid] @reverse .
<annualIncome>: float .
<city>: string @index(exact) .
<firstName>: string @index(exact) .
<lastName>: string @index(exact) .
<street>: string @index(exact) .
<xid>: string @index(hash) .
```

#### The Upsert predicate

You can upload the files individually using the live loader (`dgraph live`) with the `-U` or `--upsertPredicate` option. 
Each file has records with external keys for customers (e.g., `my.org/customer/1`) and addresses (e.g., `my.org/customer/1/address/1`). 

The schema has the required fields in addition to a field named `xid`. This field will be used to hold the external key value. Please note that there's a `hash` index for the `xid` field. You will be using this `xid` field as the "Upsert" predicate (`-U` option) and pass it as an argument to the `dgraph live` command. The live loader uses the predicate's content provided by the `-U` option (`xid` in this case) to identify and update the corresponding Dgraph node. In case the corresponding Dgraph node does not exist, the live loader will create a new node.

**File** `customerNames.rdf` - Basic information like customer's first and last name:

```
<my.org/customer/1>       <firstName>  "John"     .
<my.org/customer/1>       <lastName>  "Doe"     .
<my.org/customer/2>       <firstName>  "James"     .
<my.org/customer/2>       <lastName>  "Doe"     .
```

You can load the customer information with the following command:

```sh
dgraph live -f customerNames.rdf -U "xid"
```

Next, you can inspect the loaded data:  

```graphql
{
  q1(func: has(firstName)){
    uid
    firstName
    lastName
    annualIncome
    xid
    address{
      uid
      street
      xid
    }
  }  
}
```

The query will return the newly created Dgraph nodes as shown below.

```json
"q1": [
  {
    "uid": "0x14689d2",
    "firstName": "John",
    "lastName": "Doe",
    "xid": "my.org/customer/1"
  },
  {
    "uid": "0x14689d3",
    "firstName": "James",
    "lastName": "Doe",
    "xid": "my.org/customer/2"
  }
] 
```

You can see the new customer added with name information and the contents of the `xid` field. 
The `xid` field holds a reference to the externally provided id.

**File** `customer_income.rdf` - Income information about the customer:

```
<my.org/customer/1>       <annualIncome> "90000"    .
<my.org/customer/2>       <annualIncome> "75000"    .
```

You can load the income information by running:

```sh
dgraph live -f customer_income.rdf -U "xid"
```

Now you can execute a query to check the income data:

```graphql
{
  q1(func: has(firstName)){
    uid
    firstName
    lastName
    annualIncome
    xid
    address{
      uid
      street
      city
      xid
    }
  }  
}
```

Note that the corresponding nodes have been correctly updated with the `annualIncome` attribute.

```json
"q1": [
  {
    "uid": "0x14689d2",
    "firstName": "John",
    "lastName": "Doe",
    "annualIncome": 90000,
    "xid": "my.org/customer/1"
  },
  {
    "uid": "0x14689d3",
    "firstName": "James",
    "lastName": "Doe",
    "annualIncome": 75000,
    "xid": "my.org/customer/2"
  }
]   
```

**File** `customer_address.rdf` - Address information:

```
<my.org/customer/1>     <address> <my.org/customer/1/address/1>    .
<my.org/customer/1/address/1>  <street> "One High Street" .
<my.org/customer/1/address/1>  <city> "London" .
<my.org/customer/2>        <address> <my.org/customer/2/address/1>   .
<my.org/customer/2/address/1>  <street> "Two Main Street" .
<my.org/customer/2/address/1>  <city> "New York" .
<my.org/customer/2>     <address> <my.org/customer/2/address/2>   .
<my.org/customer/2/address/2>  <street> "Ten Main Street" .
<my.org/customer/2/address/2>  <city> "Mumbai" .
```

You can extend the same approach to update `uid` predicates. 
To load the addresses linked to customers, you can launch the live loader as below. 

```sh
dgraph live -f customer_address.rdf -U "xid"
```

You can check the output of the query:

```graphql
{
  q1(func: has(firstName)){
    uid
    firstName
    lastName
    annualIncome
    xid
    address{
      uid
      street
      xid
    }
  }
}
```

The addresses are correctly added as a `uid` predicate in the respective customer nodes.

```json
"q1": [
  {
    "uid": "0x14689d2",
    "firstName": "John",
    "lastName": "Doe",
    "annualIncome": 90000,
    "xid": "my.org/customer/1",
    "address": [
      {
        "uid": "0x1945bb6",
        "street": "One High Street",
        "city": "London",
        "xid": "my.org/customer/1/address/1"
      }
    ]
  },
  {
    "uid": "0x14689d3",
    "firstName": "James",
    "lastName": "Doe",
    "annualIncome": 75000,
    "xid": "my.org/customer/2",
    "address": [
      {
        "uid": "0x1945bb4",
        "street": "Two Main Street",
        "city": "New York",
        "xid": "my.org/customer/2/address/1"
      },
      {
        "uid": "0x1945bb5",
        "street": "Ten Main Street",
        "city": "Mumbai",
        "xid": "my.org/customer/2/address/2"
      }
    ]
  }
]
```
