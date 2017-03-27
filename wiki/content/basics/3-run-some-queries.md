+++
title="Step 3: Run Some Queries"
section = "basics"
categories = ["basics"]
weight=1
slug="run-some-queries"

[menu.main]
    url = "run-some-queries"
    parent = "basics"

+++


{{ Tip | From v0.7.3,  a user interface is available at `http://localhost:8080` from the browser to run mutations and visualise  results from the queries.}}

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
