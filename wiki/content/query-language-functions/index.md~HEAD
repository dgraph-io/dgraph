+++
title = "Functions"
date = "2017-03-20T18:58:23+11:00"
icon = "<b>X. </b>"
chapter = true
next = "/next/path"
prev = "/prev/path"
weight = 0

+++


# Functions

{{Note|Functions can only be applied to [[#Indexing|indexed attributes]]. Ensure that you specify that in the [[#Schema|schema file]].}}

## Term matching

### AllOf

AllOf function will search for entities which have all of one or more terms specified. In essence, this is an intersection of entities containing the specified terms; the ordering does not matter. It follows this syntax:
`allof(predicate, "space-separated terms")`

#### Usage as Filter

Suppose we want the films of Steven Spielberg that contain the word `indiana` and `jones`.
{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.06pj8) {
    name.en
    director.film @filter(allof(name.en, "jones indiana"))  {
      name.en
    }
  }
}
```
||
```
{
  me(id: m.06pj8) {
    name@en
    director.film @filter(allof(name, "jones indiana"))  {
      name@en
    }
  }
}
```
|}

`allof` tells Dgraph that the matching films' `name.en` have to contain both the words "indiana" and "jones". Here is the response.

```
{
  "me": [
    {
      "director.film": [
        {
          "_uid_": "0xc17b416e58b32bb",
          "name": "Indiana Jones and the Temple of Doom"
        },
        {
          "_uid_": "0x7d0807a6740c25dc",
          "name": "Indiana Jones and the Kingdom of the Crystal Skull"
        },
        {
          "_uid_": "0xa4c4cc65751e98e7",
          "name": "Indiana Jones and the Last Crusade"
        },
        {
          "_uid_": "0xd1c161bed9769cbc",
          "name": "Indiana Jones and the Raiders of the Lost Ark"
        }
      ],
      "name": "Steven Spielberg"
    }
  ]
}
```

#### Usage at root

In the following example, we list all the entities (in this case all films) that have both terms "jones" and "indiana". Moreover, for each entity, we query their film genre and names.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(func:allof(name.en, "jones indiana")) {
    name.en
    genre {
      name.en
    }
  }
}
```
||
```
{
  me(func:allof(name, "jones indiana")) {
    name@en
    genre {
      name@en
    }
  }
}
```
|}


Here is a part of the response.

```
{
  "me": [
    {
      "genre": [
        {
          "name": "Adventure Film"
        },
        {
          "name": "Horror"
        }
      ],
      "name": "The Adventures of Young Indiana Jones: Masks of Evil"
    },
    {
      "genre": [
        {
          "name": "War film"
        },
        {
          "name": "Adventure Film"
        }
      ],
      "name": "The Adventures of Young Indiana Jones: Adventures in the Secret Service"
    },
    ...
    {
      "genre": [
        {
          "name": "Comedy"
        }
      ],
      "name": "The Adventures of Young Indiana Jones: Espionage Escapades"
    }
  ]
}
```

### AnyOf

AnyOf function will search for entities which have any of two or more terms specified. In essence, this is a union of entities containing the specified terms. Again, the ordering does not matter. It follows this syntax: `anyof(predicate, "space-separated terms")`

#### Usage as filter

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.06pj8) {
    name.en
    director.film @filter(anyof(name.en, "war spies"))  {
      _uid_
      name.en
    }
  }
}
```
||
```
{
  me(id: m.06pj8) {
    name@en
    director.film @filter(anyof(name, "war spies"))  {
      _uid_
      name@en
    }
  }
}
```
|}


```
{
  "me": [
    {
      "director.film": [
        {
          "_uid_": "0x38160fa42cf3f4c9",
          "name": "War Horse"
        },
        {
          "_uid_": "0x39d8574f26521fcc",
          "name": "War of the Worlds"
        },
        {
          "_uid_": "0xd915cb0eb9ad47c0",
          "name": "Bridge of Spies"
        }
      ],
      "name": "Steven Spielberg"
    }
  ]
}
```

#### Usage at root

We can look up films that contain either the word "passion" or "peacock". Surprisingly many films satisfy this criteria. We will query their name and their genres.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(func:anyof(name.en, "passion peacock")) {
    name.en
    genre {
      name.en
    }
  }
}
```
||
```
{
  me(func:anyof(name, "passion peacock")) {
    name@en
    genre {
      name@en
    }
  }
}
```
|}


```
{
  "me": [
    {
      "name": "Unexpected Passion"
    },
    {
      "genre": [
        {
          "name": "Drama"
        },
        {
          "name": "Silent film"
        }
      ],
      "name": "The New York Peacock"
    },
    {
      "genre": [
        {
          "name": "Drama"
        },
        {
          "name": "Romance Film"
        }
      ],
      "name": "Passion of Love"
    },
    ...
    {
      "genre": [
        {
          "name": "Crime Fiction"
        },
        {
          "name": "Comedy"
        }
      ],
      "name": "The Passion of the Reefer"
    }
  ]
}
```

Note that the first result with the name "Unexpected Passion" is either not a film entity, or it is a film entity with no genre.

##Inequality
### Type Values
The following [[#Scalar_Types|scalar types]] can be used in inequality functions.

* int
* float
* string
* date
* datetime

###Less than or equal to
`leq` is used to filter or obtain UIDs whose value for a predicate is less than or equal to a given value.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
    me(id: m.06pj8) {
      name.en
      director.film @filter(leq("initial_release_date", "1970-01-01"))  {
          initial_release_date
          name.en
      }
    }
}
```
||
```
{
    me(id: m.06pj8) {
      name@en
      director.film @filter(leq("initial_release_date", "1970-01-01"))  {
          initial_release_date
          name@en
      }
    }
}
```
|}

This query would return the name and release date of all the movies directed by on or Steven Spielberg before 1970-01-01.
```
{
    "me": [
        {
            "director.film": [
                {
                    "initial_release_date": "1964-03-24",
                    "name": "Firelight"
                },
                {
                    "initial_release_date": "1968-12-18",
                    "name": "Amblin"
                },
                {
                    "initial_release_date": "1967-01-01",
                    "name": "Slipstream"
                }
            ],
            "name": "Steven Spielberg"
        }
    ]
}
```

###Greater than or equal to
`geq` is used to filter or obtain UIDs whose value for a predicate is greater than or equal to a given value.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.06pj8) {
    name.en
    director.film @filter(geq("initial_release_date", "2008"))  {
        Release_date: initial_release_date
        Name: name.en
    }
  }
}
```
||
```
{
  me(id: m.06pj8) {
    name@en
    director.film @filter(geq("initial_release_date", "2008"))  {
        Release_date: initial_release_date
        Name: name@en
    }
  }
}
```
|}

This query would return Name and Release date of movies directed by Steven Spielberg after 2010.
```
{
    "me": [
        {
            "director.film": [
                {
                    "Name": "War Horse",
                    "Release_date": "2011-12-04"
                },
                {
                    "Name": "Indiana Jones and the Kingdom of the Crystal Skull",
                    "Release_date": "2008-05-18"
                },
                {
                    "Name": "Lincoln",
                    "Release_date": "2012-10-08"
                },
                {
                    "Name": "Bridge of Spies",
                    "Release_date": "2015-10-16"
                },
                {
                    "Name": "The Adventures of Tintin: The Secret of the Unicorn",
                    "Release_date": "2011-10-23"
                }
            ],
            "name": "Steven Spielberg"
        }
    ]
}
```

###Less than, greater than, equal to

Above, we have seen the usage of `geq` and `leq`. You can also use `gt` for "strictly greater than" and `lt` for "strictly less than" and `eq` for "equal to".

## Geolocation
{{Note| Geolocation functions support only polygons and points as of now. Also, polygons with holes are replaced with the outer loop ignoring any holes.}}
The data used for testing the geo functions can be found in [benchmarks repository](https://github.com/dgraph-io/benchmarks/blob/master/data/sf.tourism.gz). You will need to [[#Indexing | index]] `loc` predicate with type `geo` before loading the data for these queries to work.
### Near

`Near` returns all entities which lie within a specified distance from a given point. It takes in three arguments namely
the predicate (on which the index is based), geo-location point and a distance (in metres).

```
{
  tourist( near(loc, [-122.469829, 37.771935], 1000) ) {
    name
  }
}
```

This query returns all the entities located within 1000 metres from the [http://bl.ocks.org/d/2ba9f626cb7be1bcc012be1dc7db40ff specified point] in geojson format.
```
{
    "tourist": [
        {
            "name": "National AIDS Memorial Grove"
        },
        {
            "name": "Japanese Tea Garden"
        },
        {
            "name": "Peace Lantern"
        },
        {
            "name": "Steinhart Aquarium"
        },
        {
            "name": "De Young Museum"
        },
        {
            "name": "Morrison Planetarium"
        },
         .
         .
        {
            "name": "San Francisco Botanical Garden"
        },
        {
            "name": "Buddha"
        }
    ]
}
```

### Within

`Within` returns all entities which completely lie within the specified region. It takes in two arguments namely the predicate (on which the index is based) and geo-location region.

```
{
  tourist(within(loc, [[-122.47266769409178, 37.769018558337926 ], [ -122.47266769409178, 37.773699921075135 ], [ -122.4651575088501, 37.773699921075135 ], [ -122.4651575088501, 37.769018558337926 ], [ -122.47266769409178, 37.769018558337926]] )) {
    name
  }
}
```
This query returns all the entities (points/polygons) located completely within the [http://bl.ocks.org/d/b81a6589fa9639c9424faad778004dae specified polygon] in geojson format.
```
{
    "tourist": [
        {
            "name": "Japanese Tea Garden"
        },
        {
            "name": "Peace Lantern"
        },
        {
            "name": "Rose Garden"
        },
        {
            "name": "Steinhart Aquarium"
        },
        {
            "name": "De Young Museum"
        },
        {
            "name": "Morrison Planetarium"
        },
        {
            "name": "Spreckels Temple of Music"
        },
        {
            "name": "Hamon Tower"
        },
        {
            "name": "Buddha"
        }
    ]
}
```
{{Note| The containment check for polygons are approximate as of v0.7.1.}}

### Contains

`Contains` returns all entities which completely enclose the specified point or region. It takes in two arguments namely the predicate (on which the index is based) and geo-location region.

```
{
  tourist(contains(loc, [ -122.50326097011566, 37.73353615592843 ] )) {
    name
  }
}
```
This query returns all the entities that completely enclose the [http://bl.ocks.org/d/7218dd34391fac518e3516ea6fc1b6b1 specified point] (or polygon) in geojson format.
```
{
    "tourist": [
        {
            "name": "San Francisco Zoo"
        },
        {
            "name": "Flamingo"
        }
    ]
}
```

### Intersects

`Intersects` returns all entities which intersect with the given polygon. It takes in two arguments namely the predicate (on which the index is based) and geo-location region.

```
{
  tourist(intersects(loc, [[-122.503325343132, 37.73345766902749 ], [ -122.503325343132, 37.733903134117966 ], [ -122.50271648168564, 37.733903134117966 ], [ -122.50271648168564, 37.73345766902749 ], [ -122.503325343132, 37.73345766902749]] )) {
    name
  }
}
```
This query returns all the entities that intersect with the [http://bl.ocks.org/d/2ed3361a25442414e15d7eab88574b67 specified polygon/point] in geojson format.
```
{
    "tourist": [
        {
            "name": "San Francisco Zoo"
        },
        {
            "name": "Flamingo"
        }
    ]
}
```
