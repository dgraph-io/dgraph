+++
title = "Geolocation"
weight = 4
[menu.main]
    parent = "graphql-queries"
    name = "Geolocation"
+++

In this article you'll learn how to use Geolocation queries in GraphQL when running a Dgraph database.

{{% notice "note" %}}
A predicate which has a geolocation type can store data for a `Point`, a `Polygon` or a `MultiPolygon`.
See the <a href="{{< relref "graphql/schema/types.md#custom-scalars">}}">Types section</a> for more information.
{{% /notice %}}

## Geolocation example

Assuming that you have a `Hotel` type that has a `location` and an `area`:

```graphql
type Hotel {
  id: ID!
  name: String!
  location: Point
  area: Polygon
}
```

Dgraph will automatically add an index to the required fields in the generated schema:

```graphql
Hotel.location: geo @index(geo) .
Hotel.area: geo @index(geo) .
```

## Mutations

Following the previous example, the `AddHotelInput` and `HotelPatch` mutations will contain `location` and `area`.

```graphql
input AddHotelInput {
  name: String!
  location: Point
  area: Polygon
}

input HotelPatch {
  name: String!
  location: Point
  area: Polygon
}
```

## Queries

You can generate the Geolocation queries as part of the `Hotel` filter so that it can be combined with other filters.

```graphql
input NearFilter {
    Distance: Float!
    Coordinate: Point!
}

input WithinFilter {
    polygon: Polygon!
}

input ContainsFilter {
    # The user should be giving one of these.
    point: Point
    polygon: Polygon
}

input IntersectsFilter {
    # The user should be giving one of these.
    polygon: Polygon
    mulitPolygon: MultiPolygon
}

input PointGeoFilter {
    near: NearFilter
    within: WithinFilter
}

input PolygonGeoFilter {
    near: NearFilter
    within: WithinFilter
    contains: ContainsFilter
    intersects: IntersectsFilter
}

input HotelFilter {
    location: PointGeoFilter
    area: PolygonGeoFilter
    and: HotelFilter
    or: HotelFilter
    not: HotelFilter
}
```

## Filters

Geolocation supports four filter options:
- `near`
- `within`
- `contains`
- `intersects`

Check the <a href="{{< relref "graphql/schema/search.md#geolocation">}}">Search and Filtering</a> section for details and examples on how to use the Geolocation filters.
