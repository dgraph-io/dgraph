+++
title = "Geolocation"
weight = 4
[menu.main]
    parent = "graphql-queries"
    name = "Geolocation"
+++

In this article you'll learn how to use Geolocation queries in GraphQL when running a Dgraph database.

## Custom scalars

In Dgraph, a predicate which has geo-type can store data for a `Point`, a `Polygon` or a `MultiPolygon`. 

{{% notice "note" %}}
since GraphQL is strongly typed, a field inside a `type` can only store one type of data.
{{% /notice %}}

### Point

```graphql
type Point {
  Latitude: Float!
  Longitude: Float!
}
```

### PointList

```graphql
type PointList {
  Points: [Point!]!
}
```

### Polygon

```graphql
type Polygon {
  Coordinates: [PointList!]!
}
```

### MultiPolygon

```graphql
type MultiPolygon {
  Coordinates: [Polygon!]!
}
```

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


### near

The `near` function matches all entities where the location given by `predicate` is within a distance `meters` from a geojson coordinate `[long, lat]`.

```graphql
{
  tourist(func: near(loc, [-122.469829, 37.771935], 1000) ) {
    name
  }
}
```

```graphql
queryHotel(filter: {
    location: { 
        near: {
            coordinate: {
                latitute: 37.771935, 
                longitude: -122.469829
            }, 
            distance: 1000
        }
    }
}) {
  name
}
```

### within

The `within` function matches all entities where the location given by `predicate` lies within a polygon specified by the geojson `coordinates` array.

{{% notice "tip" %}}
the `within` query allows searching for all geo entities (`point`, `polygon`) within a polygon; hence they are generated for all types.
{{% /notice %}}


```graphql
{
  tourist(func: within(loc, [[[....]]] )) {
    name
  }
}
```

```graphql
queryHotel(filter: {
    location: { 
        within: {
            polygon: {
                coordinates: [[[....]]],
            }
        }
    }
}) {
  name
}
```

### contains

The `contains` function matches all entities where the polygon describing a location given by `predicate` contains a geojson coordinate `[long, lat]` or a given geojson `polygon`.

{{% notice "tip" %}}
the `ContainsFilter` is only generated for `Polygon` and `MultiPolygon`.
{{% /notice %}}


```graphql
{
  tourist(func: contains(loc, [ -122.50326097011566, 37.73353615592843 ] )) {
    name
  }
}
```


```graphql
queryHotel(filter: {
    area: { 
        contains: {
            point: {
                coordinates: [],
            }
        }
    }
}) {
  name
}

```

### intersects 

The `intersects ` function matches all entities where the polygon describing a location given by `predicate` intersects a given geojson `polygon`.

{{% notice "tip" %}}
the `IntersectsFilter` is only generated for `Polygon` and `MultiPolygon`.
{{% /notice %}}

```graphql
{
  tourist(func: intersects(loc, [[[...]]] )) {
    name
  }
}
```

```graphql
queryHotel(filter: {
    area: { 
        intersects: {
            polygon: {
                coordinates: [[[...]]],
            }
        }
    }
}) {
  name
}

```
