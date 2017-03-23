+++
title = "Get Started"
date = "2017-03-20T18:58:23+11:00"
next = "/next/path"
prev = "/prev/path"
weight = 0
+++

**New to Dgraph? Here's a 5 step tutorial to get you up and running.**

## Step 1: Installation

### System Installation

You could simply install the binaries with

``` bash
$ curl https://get.dgraph.io -sSf | bash
```

That script would automatically install Dgraph for you. Once done, you can jump straight to step 2.

**Alternative:** To mitigate potential security risks, you could instead do this:
```
$ curl https://get.dgraph.io > /tmp/get.sh
$ vim /tmp/get.sh  # Inspect the script
$ sh /tmp/get.sh   # Execute the script
```

### Docker Image Installation

You may pull our Docker images [from here](https://hub.docker.com/r/dgraph/dgraph/). From terminal, just type:
```
docker pull dgraph/dgraph
```

## Step 2: Run Dgraph

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

{{% notice tip %}}
The dgraph server listens on port 8080 (unless you have mapped to another port above) with log output to the terminal.
{{% /notice %}}

## Step 3: Run some queries

{{% notice tip %}}
From v0.7.3,  a user interface is available at `http://localhost:8080` from the browser to run mutations and visualise  results from the queries.
{{% /notice %}}

Lets do a mutation which stores information about the first three releases of the the ''Star Wars'' series and one of the ''Star Trek'' movies.
```
curl localhost:8080/query -XPOST -d $'
mutation {
  set {
   _:luke <name> "Luke Skywalker" .
   _:leia <name> "Princess Leia" .
   _:han <name> "Han Solo" .
   _:lucas <name> "George Lucas" .
   _:irvin <name> "Irvin Kernshner" .
   _:richard <name> "Richard Marquand" .

   _:sw1 <name> "Star Wars: Episode IV - A New Hope" .
   _:sw1 <release_date> "1977-05-25" .
   _:sw1 <revenue> "775000000" .
   _:sw1 <running_time> "121" .
   _:sw1 <starring> _:luke .
   _:sw1 <starring> _:leia .
   _:sw1 <starring> _:han .
   _:sw1 <director> _:lucas .

   _:sw2 <name> "Star Wars: Episode V - The Empire Strikes Back" .
   _:sw2 <release_date> "1980-05-21" .
   _:sw2 <revenue> "534000000" .
   _:sw2 <running_time> "124" .
   _:sw2 <starring> _:luke .
   _:sw2 <starring> _:leia .
   _:sw2 <starring> _:han .
   _:sw2 <director> _:irvin .

   _:sw3 <name> "Star Wars: Episode VI - Return of the Jedi" .
   _:sw3 <release_date> "1983-05-25" .
   _:sw3 <revenue> "572000000" .
   _:sw3 <running_time> "131" .
   _:sw3 <starring> _:luke .
   _:sw3 <starring> _:leia .
   _:sw3 <starring> _:han .
   _:sw3 <director> _:richard .

   _:st1 <name> "Star Trek: The Motion Picture" .
   _:st1 <release_date> "1979-12-07" .
   _:st1 <revenue> "139000000" .
   _:st1 <running_time> "132" .
  }
}'
```

Now lets get the movies (and their associated information) starting with "Star Wars" and which were released after "1980".
```
curl localhost:8080/query -XPOST -d $'{
  me(func:allof("name", "Star Wars")) @filter(geq("release_date", "1980")) {
    name
    release_date
    revenue
    running_time
    director {
     name
    }
    starring {
     name
    }
  }
}' | python -m json.tool | less
```

```
{
    "me": [
        {
            "director": [
                {
                    "name": "Irvin Kernshner"
                }
            ],
            "name": "Star Wars: Episode V - The Empire Strikes Back",
            "release_date": "1980-05-21",
            "revenue": 534000000.0,
            "running_time": 124,
            "starring": [
                {
                    "name": "Han Solo"
                },
                {
                    "name": "Princess Leia"
                },
                {
                    "name": "Luke Skywalker"
                }
            ]
        },
        {
            "director": [
                {
                    "name": "Richard Marquand"
                }
            ],
            "name": "Star Wars: Episode VI - Return of the Jedi",
            "release_date": "1983-05-25",
            "revenue": 572000000.0,
            "running_time": 131,
            "starring": [
                {
                    "name": "Han Solo"
                },
                {
                    "name": "Princess Leia"
                },
                {
                    "name": "Luke Skywalker"
                }
            ]
        }
    ]
}
```

## Step 4: Advanced Queries on a larger dataset
{{< notice note >}}
Step 4 and 5 are optional. If you'd like to experiment with a larger dataset and explore more functionality, this section is for you.
{{< /notice >}}

### Download dataset
First, download the goldendata.rdf.gz dataset from [here](https://github.com/dgraph-io/benchmarks/blob/master/data/goldendata.rdf.gz) ([download](https://github.com/dgraph-io/benchmarks/raw/master/data/goldendata.rdf.gz)). Also, download the corresponding schema from [here](https://github.com/dgraph-io/benchmarks/blob/master/data/goldendata.schema) ([download](https://raw.githubusercontent.com/dgraph-io/benchmarks/master/data/goldendata.schema)). Put both files in `~/dgraph` directory, creating it if necessary using `mkdir ~/dgraph`.
```
$ mkdir -p ~/dgraph
$ cd ~/dgraph
$ wget "https://github.com/dgraph-io/benchmarks/blob/master/data/goldendata.rdf.gz?raw#true" -O goldendata.rdf.gz -q
$ wget "https://github.com/dgraph-io/benchmarks/blob/master/data/goldendata.schema?raw#true" -O goldendata.schema -q
```

### Load dataset
Start schema with the schema file.
```
$ cd ~/dgraph # The directory where you downloaded the rdf.gz and schema files.
$ dgraph --schema goldendata.schema

# Or to run it using Docker.
# Assuming you have a dgraph directory which contains goldendata.schema in your present working directory.
$ docker run -it -p 8080:8080 -v $(pwd)/dgraph:/dgraph dgraph/dgraph dgraph --bindall#true --schema#goldendata.schema
```

Load the golden dataset that you previously downloaded by running the following in another terminal:
```
$ cd ~/dgraph # The directory where you downloaded the rdf.gz and schema files.
$ dgraphloader -r goldendata.rdf.gz
...
Processing goldendata.rdf.gz
Number of mutations run   : 1121
Number of RDFs processed  : 1120879
Time spent                : MMmSS.FFFFFFFFs
RDFs processed per second : XXXXX
$
```
{{Tip|Your counts should be the same, but your statistics will vary.}}

## Step 5: Run some queries

{{ Tip | From v0.7.3 ,  a user interface is available at `http://localhost:8080` from the browser to run mutations and visualise  results from the queries.}}

{{% notice warning %}}
In versions up to v0.7.3 , special convention is used for string values with specified language. RDF N-Quad `@lang` results in appending `.lang` to predicate name, e.g. `<0x01> <name> "Алисия"@ru .` is equivalent to `<0x01> <name.ru> "Алисия" .`. See [query language documentation](https://wiki.dgraph.io) for more details.
{{% /notice %}}

### Movies by Steven Spielberg

Let's now find all the entities named "Steven Spielberg," and the movies directed by them.

{| class#"wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
curl localhost:8080/query -XPOST -d '{
  director(func:allof("name.en", "steven spielberg")) {
    name.en
    director.film (orderdesc: initial_release_date) {
      name.en
      initial_release_date
    }
  }
}
' | python -m json.tool | less
```
||
```
curl localhost:8080/query -XPOST -d '{
  director(func:allof("name", "steven spielberg")) {
    name@en
    director.film (orderdesc: initial_release_date) {
      name@en
      initial_release_date
    }
  }
}
' | python -m json.tool | less
```
|}

This query will return all the movies by the popular director Steven Spielberg, sorted by release date in descending order. The query  also return two other entities which have "Steven Spielberg" in their names.
{{Tip|You may use python or python3 equally well.}}

### Released after August 1984
Now, let's do some filtering. This time we'll only retrieve the movies which were released after August 1984. We'll sort in increasing order this time by using `orderasc`, instead of `orderdesc`.

{| class#"wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
curl localhost:8080/query -XPOST -d '{
  director(func:allof("name.en", "steven spielberg")) {
    name.en
    director.film (orderasc: initial_release_date) @filter(geq("initial_release_date", "1984-08")) {
      name.en
      initial_release_date
    }
  }
}
' | python -m json.tool | less
```
||
```
curl localhost:8080/query -XPOST -d '{
  director(func:allof("name", "steven spielberg")) {
    name@en
    director.film (orderasc: initial_release_date) @filter(geq("initial_release_date", "1984-08")) {
      name@en
      initial_release_date
    }
  }
}
' | python -m json.tool | less
```
|}



### Released in 1990s
We'll now add an AND filter using `AND` and find only the movies released in the 90s.
{| class#"wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
curl localhost:8080/query -XPOST -d '{
  director(func:allof("name.en", "steven spielberg")) {
    name.en
    director.film (orderasc: initial_release_date) @filter(geq("initial_release_date", "1990") AND leq("initial_release_date", "2000")) {
      name.en
      initial_release_date
    }
  }
}
' | python -m json.tool | less
```
||
```
curl localhost:8080/query -XPOST -d '{
  director(func:allof("name", "steven spielberg")) {
    name@en
    director.film (orderasc: initial_release_date) @filter(geq("initial_release_date", "1990") AND leq("initial_release_date", "2000")) {
      name@en
      initial_release_date
    }
  }
}
' | python -m json.tool | less
```
|}


### Released since 2016
So far, we've been retrieving film titles using the name of the director. Now, we'll start with films released since 2016, and their directors. To make things interesting, we'll only retrieve the director name, if it matches any of ''travis'' or ''knight''. In addition, we'll also alias `initial_release_date` to `release`. This will make the result look better.

{| class#"wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
curl localhost:8080/query -XPOST -d '{
  films(func:geq("initial_release_date", "2016")) {
    name: name.en
    release: initial_release_date
    directed_by @filter(anyof("name.en", "travis knight")) {
      name: name.en
    }
  }
}
' | python -m json.tool | less
```
||
```
curl localhost:8080/query -XPOST -d '{
  films(func:geq("initial_release_date", "2016")) {
    name@en
    release: initial_release_date
    directed_by @filter(anyof("name", "travis knight")) {
      name@en
    }
  }
}
' | python -m json.tool | less
```
|}


This should give you an idea of some of the queries Dgraph is capable of. A wider range of queries can been found in the [[Query Language]] section.

## Need Help
* Please use [discuss.dgraph.io](https://discuss.dgraph.io) for questions, feature requests and discussions.
* Please use [Github Issues](https://github.com/dgraph-io/dgraph/issues) if you encounter bugs or have feature requests.
* You can also join our [Slack channel](http://slack.dgraph.io).

## Troubleshooting

### 1. Docker: Error initialising postings store

One of the things to try would be to open bash in the container and try to run Dgraph from within it.

```
$ docker run -it dgraph/dgraph bash
# Now that you are within the container
$ dgraph
```

If Dgraph runs for you that indicates there could be something wrong with mounting volumes.
