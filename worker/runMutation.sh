#!/bin/bash

curl "http://localhost:8080/query" -XPOST -d $'mutation {
	set {
		<film.director.film>	<http://www.w3.org/2000/01/rdf-schema#domain>	<film.director>	.
		<film.director.film>	<http://www.w3.org/2000/01/rdf-schema#range>	<film.film>	.
		<film.director.film>	<type.object.name>	"Films ..."@en	.
		<film.director.film>	<type.object.name>	"Films ..."	.
		<film.director.film>	<type.object.name>	"Peliculas ..."@es	.
		<film.director.film>	<type.object.value>	"Some \\"value\\""	.
		<film.director.film>	<type.property.expected_type>	<film.film>	.
		<film.director.film>	<type.property.schema>	<film.director>	.
		<x> <value> <y> .
		<x> <name> "a name" .
		<y> <name> "some name" .
	}
}'

sleep 3
echo ""

curl "http://localhost:8080/query" -XPOST -d $'
{
	query(func:anyofterms(name, "name")){
		name
		value @filter(regexp(name, "^[a-z A-Z]+$")) {
			name
		}
	}
}
'


# <film.director.film>	<http://www.w3.org/2000/01/rdf-schema#domain>	<film.director>	.
# <film.director.film>	<http://www.w3.org/2000/01/rdf-schema#range>	<film.film>	.
# <film.director.film>	<type.object.name>	"Films test"	.
# <film.director.film>	<type.object.name>	"Films ..."@en	.
# <film.director.film>	<type.object.name>	"Peliculas ..."@es	.
# <film.director.film>	<type.object.value>	"Some value"	.
# <film.director.film>	<type.property.expected_type>	<film.film>	.
# <film.director.film>	<type.property.schema>	<film.director>	.

# <0xdada9dc85b146592> <type.object.name> "Films ..."@en .
# <0xdada9dc85b146592> <type.object.name> "Peliculas ..."@es .
# <0xdada9dc85b146592> <type.object.name> "Films test"^^<xs:string> .
# <0xdada9dc85b146592> <type.object.value> "Some value" .
# <0xdada9dc85b146592> <type.property.schema> <0xbc7552702ff50d6> .
# <0xdada9dc85b146592> <type.property.expected_type> <0xa51fa2739211dcba> .
# <0xdada9dc85b146592> <http://www.w3.org/2000/01/rdf-schema#range> <0xa51fa2739211dcba> .
# <0xdada9dc85b146592> <http://www.w3.org/2000/01/rdf-schema#domain> <0xbc7552702ff50d6> .

sleep 1

# curl "http://127.0.0.1:8080/admin/backup" 

echo ""
