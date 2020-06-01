Install the latest Dgraph binary from source
```
curl https://get.dgraph.io -sSf | bash
```


Create a config.properties file that has the following options (values should not be in quotes):
```
user = <the user for logging in to the SQL database>
password = <the password for logging in to the SQL database>
db = <the SQL database to be migrated>
```


Export the SQL database into a schema and RDF file, e.g. the schema.txt and sql.rdf file below
```
dgraph migrate --config config.properties --output_schema schema.txt --output_data sql.rdf
```

If you are connecting to a remote DB (something hosted on AWS, GCP, etc...), you need to pass the following flags
```
-- host <the host of your remote DB>
-- port <if anything other than 3306>


Import the data into Dgraph with the live loader (the example below is connecting to the Dgraph zero and alpha servers running on the default ports)
```
dgraph live -z localhost:5080 -a localhost:9080 --files sql.rdf --format=rdf --schema schema.txt
```
