+++
date = "2017-03-20T22:25:17+11:00"
title = "Language and RDF Types"
weight = 5
[menu.main]
    parent = "mutations"
+++

RDF N-Quad allows specifying a language for string values and an RDF type.  Languages are written using `@lang`. For example
```
<0x01> <name> "Adelaide"@en .
<0x01> <name> "Аделаида"@ru .
<0x01> <name> "Adélaïde"@fr .
<0x01> <dgraph.type> "Person" .
```
See also [how language strings are handled in queries]({{< relref "query-language/graphql-fundamentals.md#language-support" >}}).

RDF types are attached to literals with the standard `^^` separator.  For example
```
<0x01> <age> "32"^^<xs:int> .
<0x01> <birthdate> "1985-06-08"^^<xs:dateTime> .
```

The supported [RDF datatypes](https://www.w3.org/TR/rdf11-concepts/#section-Datatypes) and the corresponding internal type in which the data is stored are as follows.

| Storage Type                                                    | Dgraph type     |
| -------------                                                   | :------------:   |
| &#60;xs:string&#62;                                             | `string`         |
| &#60;xs:dateTime&#62;                                           | `dateTime`       |
| &#60;xs:date&#62;                                               | `datetime`       |
| &#60;xs:int&#62;                                                | `int`            |
| &#60;xs:integer&#62;                                            | `int`            |
| &#60;xs:boolean&#62;                                            | `bool`           |
| &#60;xs:double&#62;                                             | `float`          |
| &#60;xs:float&#62;                                              | `float`          |
| &#60;geo:geojson&#62;                                           | `geo`            |
| &#60;xs:password&#62;                                           | `password`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#string&#62;           | `string`         |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#dateTime&#62;         | `dateTime`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#date&#62;             | `dateTime`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#int&#62;              | `int`            |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#positiveInteger&#62;  | `int`            |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#integer&#62;          | `int`            |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#boolean&#62;          | `bool`           |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#double&#62;           | `float`          |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#float&#62;            | `float`          |


See the section on [RDF schema types]({{< relref "query-language/schema.md#rdf-types" >}}) to understand how RDF types affect mutations and storage.
