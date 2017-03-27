+++
title="Step 2: Run Dgraph"
section = "basics"
categories = ["basics"]
weight=1
slug="run-dgraph"

[menu.main]
    url = "run-dgraph"
    parent = "basics"

+++

We will be running Dgraph using the following schema for demonstration. You can always run Dgraph even without a schema.
```
scalar (
  name: string @index
  release_date: date @index
  revenue: float
  running_time: int
)
```

To download the schema file run
```
$ wget "https://raw.githubusercontent.com/dgraph-io/benchmarks/master/data/starwars.schema?raw#true" -O starwars.schema -q
```

### Using System Installation
Follow this command to run Dgraph:
```
$ dgraph --schema starwars.schema
```

### Using Docker

If you wan't to persist the data while you play around with Dgraph then you should mount the `dgraph` volume.

```
# Assuming you have a dgraph directory which contains starwars.schema file.
$ docker run -it -p 8080:8080 -v $(pwd)/dgraph:/dgraph dgraph/dgraph dgraph --bindall#true --schema#starwars.schema

# Or to map to custom port
# Mapping port 8080 from within the container to 9090  of the instance
$ docker run -it -p 9090:8080 -v $(pwd)/dgraph:/dgraph dgraph/dgraph dgraph --bindall#true --schema#starwars.schema
```

{{Tip|The dgraph server listens on port 8080 (unless you have mapped to another port above) with log output to the terminal.}}
