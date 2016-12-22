package types

import "github.com/dgraph-io/dgraph/query/graph"

// Value represents a value sent in a mutation.
type Value *graph.Value

// Int returns an int graph.Value
func Int(val int32) Value {
	return &graph.Value{&graph.Value_IntVal{val}}
}

// Float returns an double graph.Value
func Float(val float64) Value {
	return &graph.Value{&graph.Value_DoubleVal{val}}
}

// Str returns an string graph.Value
func Str(val string) Value {
	return &graph.Value{&graph.Value_StrVal{val}}
}

// Bytes returns an byte array graph.Value
func Bytes(val []byte) Value {
	return &graph.Value{&graph.Value_BytesVal{val}}
}

// Bool returns an bool graph.Value
func Bool(val bool) Value {
	return &graph.Value{&graph.Value_BoolVal{val}}
}

// Geo returns a geo graph.Value
func Geo(val []byte) Value {
	return &graph.Value{&graph.Value_GeoVal{val}}
}

func Date(val []byte) Value {
	return &graph.Value{&graph.Value_DateVal{val}}
}

func Datetime(val []byte) Value {
	return &graph.Value{&graph.Value_DatetimeVal{val}}
}
