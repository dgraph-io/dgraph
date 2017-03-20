+++
date = "2017-03-20T19:19:14+11:00"
draft = true
title = "query language schema"

+++

Schema is used to specify the scalar types of the predicates and what fields constitute an object type. This schema would be used for type checking, result validation, type coercion.

## Scalar Types 

Scalar types are specified with `scalar` keyword.

{| class="wikitable"
|-
! Dgraph Type
! Go type
|-
| `int`
| int32
|-
| `float`
| float
|-
| `string`
| string
|-
| `bool`
| bool
|-
| `id`
|  string
|-
| `date`
|  time.Time (only day, month, year fields are valid. It could be of the form yyyy-mm-dd or yyyy-mm or yyyy)
|-
| `datetime`
|  time.Time (RFC3339 format [Optional timezone] eg: 2006-01-02T15:04:05.999999999+10:00 or 2006-01-02T15:04:05.999999999)
|-
| `geo`
|  [go-geom](https://github.com/twpayne/go-geom)
|-
| `uid`
|  uint64 
|}
{{Note|uid type is used to denote objects though it internally uses uint64.}}

To declare a field `age` as `int`, this line has to be included in the schema file `scalar age: int`.

## Object Types 
Object types in the schema are defined using the `type` keyword. For example, to declare an object  `Person` we add the following snippet in the schema file. All objects are of `uid` type which denotes a `uint64`.

```
type person {
  name: string
  age: int
  strength: float
  profession: string
  friends: uid
  relatives: uid
} 
```

The object can have scalar fields which contain values and object fields which link to other nodes in the graph. In the above declaration name, age, strength, profession are scalar fields which would have values of specified types and friends and relatives are of person object type which would link to other nodes in the graph.

The node could have zero or more entities linked to the object fields (In this example, `friends` and `relatives`).

## Schema File 
A sample schema file would look as follows:

```
scalar (
  age:int
  address: string
)
type  Person {
  name: string
  age: int
  address: string
  friends: uid
}
type Actor {
  name  : string
  films: uid 
}
type Film {
  name: string
  budget: int
}
```
A schema file is passed to the server during invocation trough `--schema` flag. Some points to remember about the schema system are:
* A given field can have only one type throughout the schema (Both inside and outside object types). Example: `age` declared as `int` both using scalar and inside Person object can have only one type throughout the schema.
* Scalar fields inside the object types are also considered global scalars and need not be explicitly declared globally. In the above example, `name` is automatically inferred as `string` type.
* Mutations only check the scalar types (inside objects and global explicit definition). For example, in the given schema, any mutation that sets age would be checked for being a valid integer, any mutation that sets name would be checked for being a valid string (though `name` is not globally declared as a `string` scalar, it would be inferred from the object types).
* The returned fields are of types specified in the schema (given they were specified).
* If schema was not specified, the schema would be derived based on the first mutation for that field.  The rdf type present in the first mutation would be considered as the schema for the field.

## Indexing 

`@index` keyword at the end of a scalar field declaration in the schema file specifies that the predicate should be indexed. For example, if we want to index some fields, we should have a schema file similar to the one below.
```
scalar (
  name: string @index
  age: int @index
  address: string @index
  dateofbirth: date @index
  health: float @index
  location: geo @index
  timeafterbirth:  dateTime @index
)
```

All the scalar types except uid type can be indexed in dgraph. In the above example, we use the default tokenizer for each data type. You can specify a different tokenizer by writing `@index(tokenizerName)`. For example, for a string, you currently have a choice between two tokenizers `term` which is the default and `exact`. The `exact` tokenizer is useful when you want to do exact matching. Here is an example schema file that explicitly specify all the tokenizers being used.

```
scalar (
  name: string @index(exact)
  age: int @index(int)
  address: string @index(term)
  dateofbirth: date @index(date)
  health: float @index(float)
  location: geo @index(geo)
  timeafterbirth:  dateTime @index(datetime)
)
```

The available tokenizers are currently `term, exact, int, float, geo, date, datetime`. All of them except `exact` are the default tokenizers for their respective data types.

At times, you may want to rebuild the index for a predicate. You can achieve that by a simple GET request to the dgraph server: `admin/index?attr=yourpredicate`.

##Reverse Edges
Each graph edge is unidirectional. It points from one node to another. A lot of times,  you wish to access data in both directions, forward and backward. Instead of having to send edges in both directions, you can use the `@reverse` keyword at the end of a uid (entity) field declaration in the schema file. This specifies that the reverse edge should be automatically generated. For example, if we want to add a reverse edge for `directed_by` predicate, we should have a schema file as follows.

```
scalar (
  name.en: string @index
  directed_by: uid @reverse
)
```

This would add a reverse edge for each `directed_by` edge and that edge can be accessed by prefixing `~` with the original predicate, i.e. `~directed_by`.

In the following example, we find films that are directed by Steven Spielberg, by using the reverse edges of `directed_by`. Here is the sample query:
{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
query {
  me(id: m.06pj8) {
    name.en
    ~directed_by(first: 5) {
      name.en
    }
  }
}
```
||
```
query {
  me(id: m.06pj8) {
    name@en
    ~directed_by(first: 5) {
      name@en
    }
  }
}
```
|}


The results are:
```
{
  "me":[
    {
      "name":"Steven Spielberg",
      "~directed_by":[
        {
          "name":"Indiana Jones and the Temple of Doom"
        },
        {
          "name":"Jaws"
        },
        {
          "name":"Saving Private Ryan"
        },
        {
          "name":"Close Encounters of the Third Kind"
        },
        {
          "name":"Catch Me If You Can"
        }
      ]
    }
  ]
}
```

