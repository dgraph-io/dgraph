+++
date = "2017-03-20T19:35:35+11:00"
draft = true
title = "faq"

+++

## General 

### What is Dgraph? 
Dgraph is a distributed, low-latency, high throughput graph database, written in Go. It puts a lot of emphasis on good design, concurrency and minimizing network calls required to execute a query in a distributed environment.

### Why build Dgraph? 
We think graph databases are currently second class citizens. They are not considered mature enough to be run as the sole database, and get run alongside other SQL/NoSQL databases. Also, we're not happy with the design decisions of existing graph databases, which are either non-native or non-distributed, don't manage underlying data or suffer from performance issues.

### Why would I use Dgraph? 
If you're interested in a high-performance graph database with an emphasis on sound design, thoughtful implementation, resilience, and cutting edge technologies Dgraph is definitely something you should consider.

If you're running more than five tables in a traditional relational database managment system such as MySQL, SQL Server, or Oracle and your application requires five or more foreign keys, a graph database may be a better fit. If you're running a NoSQL database like MongoDB or Cassandra forcing you to do joins in the application layer, you should definitely take a look at moving to a graph database.

While we absolutely believe in Dgraph it's important to remember it's still young. At this stage it's ideal for internal non-user facing projects as well as for projects that you've found impossible to realize in the past due to the complexity and computational cost imposed by classic table driven systems, endless joins, or the seemingly inescapable ''curse of dimensionality''.

### Why would I not use Dgraph? 
If you're looking for a stable, mature database, Dgraph wouldn't be the right fit for you. It is at an early stage, where a lot of functionality is still being worked on, and releases might not be backward compatible.

Another thing is, if your data doesn't have graph structure, i.e., there's only one predicate, then any graph database might not be a good fit for you. A NoSQL datastore is best for key-value type storage.

### Is Dgraph production ready?
We recommend Dgraph to be used in internal projects at companies. Minor releases at this stage might not be backward compatible; so we highly recommend using [frequent backups](https://wiki.dgraph.io/Deploy#Backup).

### Is Dgraph fast?
Every other graph system that I've run it against, Dgraph has been at least a 10x factor faster. It only goes up from there. But, that's my own observations. We have a couple of folks doing a thorough benchmarking of Dgraph against Cayley and Neo4J, which we'll publish blog posts for soon. See relevant Github [issues here](https://github.com/dgraph-io/dgraph/issues?q=is%3Aissue+label%3Abenchmark).

## Internals 

### Why does Dgraph use RocksDB? 
RocksDB, as opposed to the name, isn't a database. It's an application library which helps with key-value storage on disk. Dgraph uses RocksDB to store <code>posting lists</code> on disk. It, however, doesn't rely upon RocksDB for anything else. All the data handling happens at Dgraph level. RocksDB is only an interface to disk (sort of how a file library is).

### Why doesn't Dgraph use BoltDB? 
BoltDB depends on a single global <code>RWMutex</code> lock for all reads and writes; this negatively affects concurrency of iteration and modification of posting lists for Dgraph. For this reason, we decided not to use it and instead use RocksDB. On the other hand, RocksDB supports concurrent writes and is being used in production both at Google and Facebook.

### Can Dgraph run on other databases, like Cassandra, MySQL, etc.? 
No. Dgraph stores and handles data natively to ensure it has complete control over performance and latency. The only thing between Dgraph and disk is the key-value application library, RocksDB.

## Languages and Features 

### Does Dgraph support GraphQL? 
Dgraph started with the aim to fully support GraphQL. However, as our experience with the language grew, we started hitting the seams. It couldn't support many of the features required from a language meant to interact with Graph data, and we felt some of the features were unnecessary and complicated. So, we've created a simplified and feature rich version of GraphQL. For lack of better name, we're calling GraphQL+-. You can [read more about it here](https://wiki.dgraph.io/Query_Language).

### When is Dgraph going to support Gremlin? 
Dgraph will aim to support [Gremlin](https://github.com/tinkerpop/gremlin/wiki) after v1.0. However, this is not set in stone. If our community wants Gremlin support to interact with other frameworks, like Tinkerpop, we can look into supporting it earlier.

### Is Dgraph going to support Cypher? 
If there is a demand for it, Dgraph could support [Cypher](https://neo4j.com/developer/cypher-query-language/). It would most likely be after v1.0.

### Can Dgraph support X? 
Please see Dgraph [product roadmap](https://github.com/dgraph-io/dgraph/issues/1) of what we're planning to support for v1.0. If `request X` is not part of it, please feel free to start a discussion at [discuss.dgraph.io](https://discuss.dgraph.io), or file a [Github Issue](https://github.com/dgraph-io/dgraph/issues).

## Long Term Plans 

### Will Dgraph remain open source? 
Yes. We have 2 versions of Dgraph: Community, which is under open source license. And enterprise, which is closed-source. Unlike other databases, we include running Dgraph distributedly in our community version; because we aim our open source version at young startups, who need to scale as demand grows.

### Would Dgraph be well supported? 
Yes. We're VC funded and plan to use the funds for development. We have a dedicated team of really smart engineers working on this as their full-time job. And of course, we're always open to contributions from the wider community.

### How does Dgraph plan to make money as a company? 
It's currently too early to say. It's very likely that we will offer commercially licensed plugins and paid support to interested customers. This model would enable us to continue advancing Dgraph while standing by our commitment to keeping the core project free, open, and available under the Apache 2.0 license.

### How can I contribute to Dgraph? 
We accept both code and documentation contributions. Please see [link](https://wiki.dgraph.io) for more information about how to contribute.

## Criticism 

### Dgraph is not highly available 
This is from [a reddit thread](https://www.reddit.com/r/golang/comments/5malnr/dgraph_v071_highly_available_using_raft/).
''Raft means choosing the C in CAP. "Highly Available" means choosing the A. I mean, yeah, adding consistent replication certainly means that it can be more available than something without replication, but advertising this as "highly available" is just misleading... Anything built on raft isn't (highly available).''

CAP theory talks about one edge case, which is what happens in case of a network partition. In case of network partition, Dgraph would chose consistency over availability; which makes it CP (not AP). However, this doesn't necessarily mean the entire system isn't available. Dgraph as a system is also highly-available.

This is from Wikipedia:

> There are three principles of systems design in reliability engineering which can help achieve high availability.

> - Elimination of single points of failure. This means adding redundancy to the system so that failure of a component does not mean failure of the entire system.
> - Reliable crossover. In redundant systems, the crossover point itself tends to become a single point of failure. Reliable systems must provide for reliable crossover.
> - Detection of failures as they occur. If the two principles above are observed, then a user may never see a failure. But the maintenance activity must.

**Dgraph does each of these 3 things** (if not already, then they're planned).

- We don't have a single point of failure. Each server has the same capabilities as the next.
- Even if some servers go down, the queries and writes would still succeed. The queries would automatically be re-routed to a healthy server. Dgraph does reliable crossover.
- Data is divided into shards and served by groups. Unless majority of the particular group needed for the query goes down, the user wouldn't see the failure. But, the maintainer would know about them.

Given these 3, I think I'm right to claim that Dgraph is highly available.
