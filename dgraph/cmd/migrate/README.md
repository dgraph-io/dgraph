install the Dgraph binary from source
```
go get -v github.com/dgraph-io/dgraph/dgraph
```

create a config.properties file that has the following options
```
user = <the user for logging in to the SQL database>
password = <the password for logging in to the SQL database>
db = <the SQL database to be migrated>
```

export the SQL database into a schema and RDF file, e.g. the schema.txt and sql.rdf file below
```
dgraph migrate --config config.properties --output_schema schema.txt --output_data sql.rdf
```

import the data into Dgraph with the live loader (the example below is connecting to the Dgraph zero and alpha servers running on the default ports)
```
dgraph live -z localhost:5080 -a localhost:9080 --files sql.rdf --format=rdf --schema schema.txt
```
