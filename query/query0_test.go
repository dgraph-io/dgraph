/*
 * Copyright 2015-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package query

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"

	context "golang.org/x/net/context"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/worker"

	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var passwordCache map[string]string = make(map[string]string, 2)

var ts uint64
var odch chan *intern.OracleDelta

func timestamp() uint64 {
	return atomic.AddUint64(&ts, 1)
}

func addPassword(t *testing.T, uid uint64, attr, password string) {
	value := types.ValueForType(types.BinaryID)
	src := types.ValueForType(types.PasswordID)
	encrypted, ok := passwordCache[password]
	if !ok {
		encrypted, _ = types.Encrypt(password)
		passwordCache[password] = encrypted
	}
	src.Value = encrypted
	err := types.Marshal(src, &value)
	require.NoError(t, err)
	addEdgeToTypedValue(t, attr, uid, types.PasswordID, value.Value.([]byte), nil)
}

var ps *badger.ManagedDB

func populateGraph(t *testing.T) {
	x.AssertTrue(ps != nil)
	// So, user we're interested in has uid: 1.
	// She has 5 friends: 23, 24, 25, 31, and 101
	addEdgeToUID(t, "friend", 1, 23, nil)
	addEdgeToUID(t, "friend", 1, 24, nil)
	addEdgeToUID(t, "friend", 1, 25, nil)
	addEdgeToUID(t, "friend", 1, 31, nil)
	addEdgeToUID(t, "friend", 1, 101, nil)
	addEdgeToUID(t, "friend", 31, 24, nil)
	addEdgeToUID(t, "friend", 23, 1, nil)

	addEdgeToUID(t, "school", 1, 5000, nil)
	addEdgeToUID(t, "school", 23, 5001, nil)
	addEdgeToUID(t, "school", 24, 5000, nil)
	addEdgeToUID(t, "school", 25, 5000, nil)
	addEdgeToUID(t, "school", 31, 5001, nil)
	addEdgeToUID(t, "school", 101, 5001, nil)

	addEdgeToValue(t, "name", 5000, "School A", nil)
	addEdgeToValue(t, "name", 5001, "School B", nil)

	addEdgeToUID(t, "follow", 1, 31, nil)
	addEdgeToUID(t, "follow", 1, 24, nil)
	addEdgeToUID(t, "follow", 31, 1001, nil)
	addEdgeToUID(t, "follow", 1001, 1000, nil)
	addEdgeToUID(t, "follow", 1002, 1000, nil)
	addEdgeToUID(t, "follow", 1001, 1003, nil)
	addEdgeToUID(t, "follow", 1001, 1003, nil)
	addEdgeToUID(t, "follow", 1003, 1002, nil)

	addEdgeToUID(t, "path", 1, 31, map[string]string{"weight": "0.1", "weight1": "0.2"})
	addEdgeToUID(t, "path", 1, 24, map[string]string{"weight": "0.2"})
	addEdgeToUID(t, "path", 31, 1000, map[string]string{"weight": "0.1"})
	addEdgeToUID(t, "path", 1000, 1001, map[string]string{"weight": "0.1"})
	addEdgeToUID(t, "path", 1000, 1002, map[string]string{"weight": "0.7"})
	addEdgeToUID(t, "path", 1001, 1002, map[string]string{"weight": "0.1"})
	addEdgeToUID(t, "path", 1002, 1003, map[string]string{"weight": "0.6"})
	addEdgeToUID(t, "path", 1001, 1003, map[string]string{"weight": "1.5"})
	addEdgeToUID(t, "path", 1003, 1001, map[string]string{})

	addEdgeToValue(t, "name", 1000, "Alice", nil)
	addEdgeToValue(t, "name", 1001, "Bob", nil)
	addEdgeToValue(t, "name", 1002, "Matt", nil)
	addEdgeToValue(t, "name", 1003, "John", nil)
	addEdgeToValue(t, "nick_name", 5010, "Two Terms", nil)

	addEdgeToValue(t, "alias", 23, "Zambo Alice", nil)
	addEdgeToValue(t, "alias", 24, "John Alice", nil)
	addEdgeToValue(t, "alias", 25, "Bob Joe", nil)
	addEdgeToValue(t, "alias", 31, "Allan Matt", nil)
	addEdgeToValue(t, "alias", 101, "John Oliver", nil)

	// Now let's add a few properties for the main user.
	addEdgeToValue(t, "name", 1, "Michonne", nil)
	addEdgeToValue(t, "gender", 1, "female", nil)
	addEdgeToValue(t, "full_name", 1, "Michonne's large name for hashing", nil)
	addEdgeToValue(t, "noindex_name", 1, "Michonne's name not indexed", nil)

	src := types.ValueForType(types.StringID)
	src.Value = []byte("{\"Type\":\"Point\", \"Coordinates\":[1.1,2.0]}")
	coord, err := types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData := types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 1, types.GeoID, gData.Value.([]byte), nil)
	addEdgeToTypedValue(t, "loc", 25, types.GeoID, gData.Value.([]byte), nil)

	// IntID
	data := types.ValueForType(types.BinaryID)
	intD := types.Val{types.IntID, int64(15)}
	err = types.Marshal(intD, &data)
	require.NoError(t, err)

	// FloatID
	fdata := types.ValueForType(types.BinaryID)
	floatD := types.Val{types.FloatID, float64(13.25)}
	err = types.Marshal(floatD, &fdata)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "power", 1, types.FloatID, fdata.Value.([]byte), nil)

	addEdgeToValue(t, "address", 1, "31, 32 street, Jupiter", nil)

	boolD := types.Val{types.BoolID, true}
	err = types.Marshal(boolD, &data)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "alive", 1, types.BoolID, data.Value.([]byte), nil)
	addEdgeToTypedValue(t, "alive", 23, types.BoolID, data.Value.([]byte), nil)

	boolD = types.Val{types.BoolID, false}
	err = types.Marshal(boolD, &data)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "alive", 25, types.BoolID, data.Value.([]byte), nil)
	addEdgeToTypedValue(t, "alive", 31, types.BoolID, data.Value.([]byte), nil)

	addEdgeToValue(t, "age", 1, "38", nil)
	addEdgeToValue(t, "survival_rate", 1, "98.99", nil)
	addEdgeToValue(t, "sword_present", 1, "true", nil)
	addEdgeToValue(t, "_xid_", 1, "mich", nil)

	// Now let's add a name for each of the friends, except 101.
	addEdgeToTypedValue(t, "name", 23, types.StringID, []byte("Rick Grimes"), nil)
	addEdgeToValue(t, "gender", 23, "male", nil)
	addEdgeToValue(t, "age", 23, "15", nil)

	src.Value = []byte(`{"Type":"Polygon", "Coordinates":[[[0.0,0.0], [2.0,0.0], [2.0, 2.0], [0.0, 2.0], [0.0, 0.0]]]}`)
	coord, err = types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 23, types.GeoID, gData.Value.([]byte), nil)

	addEdgeToValue(t, "address", 23, "21, mark street, Mars", nil)
	addEdgeToValue(t, "name", 24, "Glenn Rhee", nil)
	addEdgeToValue(t, "_xid_", 24, `g"lenn`, nil)
	src.Value = []byte(`{"Type":"Point", "Coordinates":[1.10001,2.000001]}`)
	coord, err = types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 24, types.GeoID, gData.Value.([]byte), nil)

	addEdgeToValue(t, "name", 110, "Alice", nil)
	addEdgeToValue(t, "_xid_", 110, "a.bc", nil)
	addEdgeToValue(t, "name", 25, "Daryl Dixon", nil)
	addEdgeToValue(t, "name", 31, "Andrea", nil)
	addEdgeToValue(t, "name", 2300, "Andre", nil)
	addEdgeToValue(t, "name", 2333, "Helmut", nil)
	src.Value = []byte(`{"Type":"Point", "Coordinates":[2.0, 2.0]}`)
	coord, err = types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 31, types.GeoID, gData.Value.([]byte), nil)

	addEdgeToValue(t, "dob_day", 1, "1910-01-01", nil)

	// Note - Though graduation is of [dateTime] type. Don't add another graduation for Michonne.
	// There is a test to check that JSON should return an array even if there is only one value
	// for attribute whose type is a list type.
	addEdgeToValue(t, "graduation", 1, "1932-01-01", nil)
	addEdgeToValue(t, "dob_day", 23, "1910-01-02", nil)
	addEdgeToValue(t, "dob_day", 24, "1909-05-05", nil)
	addEdgeToValue(t, "dob_day", 25, "1909-01-10", nil)
	addEdgeToValue(t, "dob_day", 31, "1901-01-15", nil)
	addEdgeToValue(t, "graduation", 31, "1933-01-01", nil)
	addEdgeToValue(t, "graduation", 31, "1935-01-01", nil)

	addEdgeToValue(t, "dob", 1, "1910-01-01", nil)
	addEdgeToValue(t, "dob", 23, "1910-01-02", nil)
	addEdgeToValue(t, "dob", 24, "1909-05-05", nil)
	addEdgeToValue(t, "dob", 25, "1909-01-10", nil)
	addEdgeToValue(t, "dob", 31, "1901-01-15", nil)

	addEdgeToValue(t, "age", 24, "15", nil)
	addEdgeToValue(t, "age", 25, "17", nil)
	addEdgeToValue(t, "age", 31, "19", nil)

	f1 := types.Val{Tid: types.FloatID, Value: 1.6}
	fData := types.ValueForType(types.BinaryID)
	err = types.Marshal(f1, &fData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "survival_rate", 23, types.FloatID, fData.Value.([]byte), nil)
	addEdgeToTypedValue(t, "survival_rate", 24, types.FloatID, fData.Value.([]byte), nil)
	addEdgeToTypedValue(t, "survival_rate", 25, types.FloatID, fData.Value.([]byte), nil)
	addEdgeToTypedValue(t, "survival_rate", 31, types.FloatID, fData.Value.([]byte), nil)

	// GEO stuff
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	addGeoData(t, 5101, p, "Googleplex")

	p = geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.080668, 37.426753})
	addGeoData(t, 5102, p, "Shoreline Amphitheater")

	p = geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.2527428, 37.513653})
	addGeoData(t, 5103, p, "San Carlos Airport")

	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-121.6, 37.1}, {-122.4, 37.3}, {-122.6, 37.8}, {-122.5, 38.3}, {-121.9, 38},
			{-121.6, 37.1}},
	})
	addGeoData(t, 5104, poly, "SF Bay area")
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122.06, 37.37}, {-122.1, 37.36}, {-122.12, 37.4}, {-122.11, 37.43},
			{-122.04, 37.43}, {-122.06, 37.37}},
	})
	addGeoData(t, 5105, poly, "Mountain View")
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122.25, 37.49}, {-122.28, 37.49}, {-122.27, 37.51}, {-122.25, 37.52},
			{-122.25, 37.49}},
	})
	addGeoData(t, 5106, poly, "San Carlos")

	multipoly, err := loadPolygon("testdata/nyc-coordinates.txt")
	require.NoError(t, err)
	addGeoData(t, 5107, multipoly, "New York")

	addEdgeToValue(t, "film.film.initial_release_date", 23, "1900-01-02", nil)
	addEdgeToValue(t, "film.film.initial_release_date", 24, "1909-05-05", nil)
	addEdgeToValue(t, "film.film.initial_release_date", 25, "1929-01-10", nil)
	addEdgeToValue(t, "film.film.initial_release_date", 31, "1801-01-15", nil)

	// for aggregator(sum) test
	{
		data := types.ValueForType(types.BinaryID)
		intD := types.Val{types.IntID, int64(4)}
		err = types.Marshal(intD, &data)
		require.NoError(t, err)
		addEdgeToTypedValue(t, "shadow_deep", 23, types.IntID, data.Value.([]byte), nil)
	}
	{
		data := types.ValueForType(types.BinaryID)
		intD := types.Val{types.IntID, int64(14)}
		err = types.Marshal(intD, &data)
		require.NoError(t, err)
		addEdgeToTypedValue(t, "shadow_deep", 24, types.IntID, data.Value.([]byte), nil)
	}

	// Natural Language Processing test data
	// 0x1001 is uid of interest for language tests
	addEdgeToLangValue(t, "name", 0x1001, "Badger", "", nil)
	addEdgeToLangValue(t, "name", 0x1001, "European badger", "en", nil)
	addEdgeToLangValue(t, "name", 0x1001, "European badger barger European", "xx", nil)
	addEdgeToLangValue(t, "name", 0x1001, "Borsuk europejski", "pl", nil)
	addEdgeToLangValue(t, "name", 0x1001, "Europäischer Dachs", "de", nil)
	addEdgeToLangValue(t, "name", 0x1001, "Барсук", "ru", nil)
	addEdgeToLangValue(t, "name", 0x1001, "Blaireau européen", "fr", nil)
	addEdgeToLangValue(t, "name", 0x1002, "Honey badger", "en", nil)
	addEdgeToLangValue(t, "name", 0x1003, "Honey bee", "en", nil)
	// data for bug (#945), also used by test for #1010
	addEdgeToLangValue(t, "name", 0x1004, "Артём Ткаченко", "ru", nil)
	addEdgeToLangValue(t, "name", 0x1004, "Artem Tkachenko", "en", nil)
	// data for bug (#1118)
	addEdgeToLangValue(t, "lossy", 0x1001, "Badger", "", nil)
	addEdgeToLangValue(t, "lossy", 0x1001, "European badger", "en", nil)
	addEdgeToLangValue(t, "lossy", 0x1001, "European badger barger European", "xx", nil)
	addEdgeToLangValue(t, "lossy", 0x1001, "Borsuk europejski", "pl", nil)
	addEdgeToLangValue(t, "lossy", 0x1001, "Europäischer Dachs", "de", nil)
	addEdgeToLangValue(t, "lossy", 0x1001, "Барсук", "ru", nil)
	addEdgeToLangValue(t, "lossy", 0x1001, "Blaireau européen", "fr", nil)
	addEdgeToLangValue(t, "lossy", 0x1002, "Honey badger", "en", nil)
	addEdgeToLangValue(t, "lossy", 0x1003, "Honey bee", "en", nil)

	// full_name has hash index, we need following data for bug with eq (#1295)
	addEdgeToLangValue(t, "royal_title", 0x10000, "Her Majesty Elizabeth the Second, by the Grace of God of the United Kingdom of Great Britain and Northern Ireland and of Her other Realms and Territories Queen, Head of the Commonwealth, Defender of the Faith", "en", nil)
	addEdgeToLangValue(t, "royal_title", 0x10000, "Sa Majesté Elizabeth Deux, par la grâce de Dieu Reine du Royaume-Uni, du Canada et de ses autres royaumes et territoires, Chef du Commonwealth, Défenseur de la Foi", "fr", nil)

	// regex test data
	// 0x1234 is uid of interest for regex testing
	addEdgeToValue(t, "name", 0x1234, "Regex Master", nil)
	nextId := uint64(0x2000)
	patterns := []string{"mississippi", "missouri", "mission", "missionary",
		"whissle", "transmission", "zipped", "monosiphonic", "vasopressin", "vapoured",
		"virtuously", "zurich", "synopsis", "subsensuously",
		"admission", "commission", "submission", "subcommission", "retransmission", "omission",
		"permission", "intermission", "dimission", "discommission",
	}

	for _, p := range patterns {
		addEdgeToValue(t, "value", nextId, p, nil)
		addEdgeToUID(t, "pattern", 0x1234, nextId, nil)
		nextId++
	}

	addEdgeToValue(t, "name", 240, "Andrea With no friends", nil)
	addEdgeToUID(t, "son", 1, 2300, nil)
	addEdgeToUID(t, "son", 1, 2333, nil)

	addEdgeToValue(t, "name", 2301, `Alice"`, nil)

	// Add some base64 encoded data
	addEdgeToTypedValue(t, "bin_data", 0x1, types.BinaryID, []byte("YmluLWRhdGE="), nil)

	// Data to check multi-sort.
	addEdgeToValue(t, "name", 10000, "Alice", nil)
	addEdgeToValue(t, "age", 10000, "25", nil)
	addEdgeToValue(t, "salary", 10000, "10000", nil)
	addEdgeToValue(t, "name", 10001, "Elizabeth", nil)
	addEdgeToValue(t, "age", 10001, "75", nil)
	addEdgeToValue(t, "name", 10002, "Alice", nil)
	addEdgeToValue(t, "age", 10002, "75", nil)
	addEdgeToValue(t, "salary", 10002, "10002", nil)
	addEdgeToValue(t, "name", 10003, "Bob", nil)
	addEdgeToValue(t, "age", 10003, "75", nil)
	addEdgeToValue(t, "name", 10004, "Alice", nil)
	addEdgeToValue(t, "age", 10004, "75", nil)
	addEdgeToValue(t, "name", 10005, "Bob", nil)
	addEdgeToValue(t, "age", 10005, "25", nil)
	addEdgeToValue(t, "name", 10006, "Colin", nil)
	addEdgeToValue(t, "age", 10006, "25", nil)
	addEdgeToValue(t, "name", 10007, "Elizabeth", nil)
	addEdgeToValue(t, "age", 10007, "25", nil)

	// Data to test inequality (specifically gt, lt) on exact tokenizer
	addEdgeToValue(t, "name", 3000, "mystocks", nil)
	addEdgeToValue(t, "symbol", 3001, "AAPL", nil)
	addEdgeToValue(t, "symbol", 3002, "AMZN", nil)
	addEdgeToValue(t, "symbol", 3003, "AMD", nil)
	addEdgeToValue(t, "symbol", 3004, "FB", nil)
	addEdgeToValue(t, "symbol", 3005, "GOOG", nil)
	addEdgeToValue(t, "symbol", 3006, "MSFT", nil)

	addEdgeToValue(t, "name", 3500, "", nil) // empty default name
	addEdgeToLangValue(t, "name", 3500, "상현", "ko", nil)
	addEdgeToValue(t, "name", 3501, "Alex", nil)
	addEdgeToLangValue(t, "name", 3501, "Alex", "en", nil)
	addEdgeToValue(t, "name", 3502, "", nil) // empty default name
	addEdgeToLangValue(t, "name", 3502, "Amit", "en", nil)
	addEdgeToLangValue(t, "name", 3502, "अमित", "hi", nil)
	addEdgeToLangValue(t, "name", 3503, "Andrew", "en", nil) // no default name & empty hi name
	addEdgeToLangValue(t, "name", 3503, "", "hi", nil)

	addEdgeToValue(t, "office", 4001, "office 1", nil)
	addEdgeToValue(t, "room", 4002, "room 1", nil)
	addEdgeToValue(t, "room", 4003, "room 2", nil)
	addEdgeToValue(t, "room", 4004, "", nil)
	addEdgeToUID(t, "office.room", 4001, 4002, nil)
	addEdgeToUID(t, "office.room", 4001, 4003, nil)
	addEdgeToUID(t, "office.room", 4001, 4004, nil)
}

func TestGetUID(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				name
				uid
				gender
				alive
				friend {
					uid
					name
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x1","alive":true,"friend":[{"uid":"0x17","name":"Rick Grimes"},{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19","name":"Daryl Dixon"},{"uid":"0x1f","name":"Andrea"},{"uid":"0x65"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestQueryEmptyDefaultNames(t *testing.T) {
	query := `{
	  people(func: eq(name, "")) {
		uid
		name
	  }
	}`
	js := processToFastJsonNoErr(t, query)
	// only two empty names should be retrieved as the other one is empty in a particular lang.
	require.JSONEq(t,
		`{"data":{"people": [{"uid":"0xdac","name":""}, {"uid":"0xdae","name":""}]}}`,
		js)
}

func TestQueryEmptyDefaultNameWithLanguage(t *testing.T) {
	query := `{
	  people(func: eq(name, "")) {
		name@ko:en:hi
	  }
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"people": [{"name@ko:en:hi":"상현"},{"name@ko:en:hi":"Amit"}]}}`,
		js)
}

func TestQueryNamesThatAreEmptyInLanguage(t *testing.T) {
	query := `{
	  people(func: eq(name@hi, "")) {
		name@en
	  }
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"people": [{"name@en":"Andrew"}]}}`,
		js)
}

func TestQueryNamesInLanguage(t *testing.T) {
	query := `{
	  people(func: eq(name@hi, "अमित")) {
		name@en
	  }
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"people": [{"name@en":"Amit"}]}}`,
		js)
}

func TestQueryNamesBeforeA(t *testing.T) {
	query := `{
	  people(func: lt(name, "A")) {
		uid
		name
	  }
	}`
	js := processToFastJsonNoErr(t, query)
	// only two empty names should be retrieved as the other one is empty in a particular lang.
	require.JSONEq(t,
		`{"data":{"people": [{"uid":"0xdac", "name":""}, {"uid":"0xdae", "name":""}]}}`,
		js)
}

func TestQueryCountEmptyNames(t *testing.T) {
	query := `{
	  people_empty_name(func: has(name)) @filter(eq(name, "")) {
		count(uid)
	  }
	}`
	js := processToFastJsonNoErr(t, query)
	// only two empty names should be counted as the other one is empty in a particular lang.
	require.JSONEq(t,
		`{"data":{"people_empty_name": [{"count":2}]}}`,
		js)
}

func TestQueryEmptyRoomsWithTermIndex(t *testing.T) {
	query := `{
		  offices(func: has(office)) {
			count(office.room @filter(eq(room, "")))
		  }
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"offices": [{"count(office.room)":1}]}}`,
		js)
}

func TestQueryCountEmptyNamesWithLang(t *testing.T) {
	query := `{
	  people_empty_name(func: has(name@hi)) @filter(eq(name@hi, "")) {
		count(uid)
	  }
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"people_empty_name": [{"count":1}]}}`,
		js)
}

func TestStocksStartsWithAInPortfolio(t *testing.T) {
	query := `{
	  portfolio(func: lt(symbol, "B")) {
		symbol
	  }
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"portfolio": [{"symbol":"AAPL"},{"symbol":"AMZN"},{"symbol":"AMD"}]}}`,
		js)
}

func TestFindFriendsWhoAreBetween15And19(t *testing.T) {
	query := `{
	  friends_15_and_19(func: uid(1)) {
		name
		friend @filter(ge(age, 15) AND lt(age, 19)) {
			name
			age
	    }
      }
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friends_15_and_19":[{"name":"Michonne","friend":[{"name":"Rick Grimes","age":15},{"name":"Glenn Rhee","age":15},{"name":"Daryl Dixon","age":17}]}]}}`,
		js)
}

func TestGeAge(t *testing.T) {
	query := `{
		  senior_citizens(func: ge(age, 75)) {
			name
			age
		  }
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"senior_citizens": [{"name":"Elizabeth", "age":75}, {"name":"Alice", "age":75}, {"age":75, "name":"Bob"}, {"name":"Alice", "age":75}]}}`,
		js)
}

func TestGtAge(t *testing.T) {
	query := `
    {
			senior_citizens(func: gt(age, 75)) {
				name
				age
			}
    }`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"senior_citizens":[]}}`, js)
}

func TestLeAge(t *testing.T) {
	query := `{
		  minors(func: le(age, 15)) {
			name
			age
		  }
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"minors": [{"name":"Rick Grimes", "age":15}, {"name":"Glenn Rhee", "age":15}]}}`,
		js)
}

func TestLtAge(t *testing.T) {
	query := `
    {
			minors(func: Lt(age, 15)) {
				name
				age
			}
    }`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"minors":[]}}`, js)
}

func TestGetUIDInDebugMode(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				name
				uid
				gender
				alive
				friend {
					uid
					name
				}
			}
		}
	`
	ctx := defaultContext()
	ctx = context.WithValue(ctx, "debug", "true")
	js, err := processToFastJsonCtxVars(t, query, ctx, nil)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x1","alive":true,"friend":[{"uid":"0x17","name":"Rick Grimes"},{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19","name":"Daryl Dixon"},{"uid":"0x1f","name":"Andrea"},{"uid":"0x65"}],"gender":"female","name":"Michonne"}]}}`,
		js)

}

func TestReturnUids(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				name
				uid
				gender
				alive
				friend {
					uid
					name
				}
			}
		}
	`
	js, err := processToFastJson(t, query)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x1","alive":true,"friend":[{"uid":"0x17","name":"Rick Grimes"},{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19","name":"Daryl Dixon"},{"uid":"0x1f","name":"Andrea"},{"uid":"0x65"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestGetUIDNotInChild(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				name
				uid
				gender
				alive
				friend {
					name
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x1","alive":true,"gender":"female","name":"Michonne", "friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}]}}`,
		js)
}

func TestCascadeDirective(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) @cascade {
				name
				gender
				friend {
					name
					friend{
						name
						dob
						age
					}
				}
			}
		}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"friend":[{"age":38,"dob":"1910-01-01T00:00:00Z","name":"Michonne"}],"name":"Rick Grimes"},{"friend":[{"age":15,"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"}],"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestLevelBasedFacetVarAggSum(t *testing.T) {
	query := `
		{
			friend(func: uid(1000)) {
				path @facets(L1 as weight)
				sumw: sum(val(L1))
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friend":[{"path":[{"path|weight":0.100000},{"path|weight":0.700000}],"sumw":0.800000}]}}`,
		js)
}

func TestLevelBasedFacetVarSum(t *testing.T) {
	query := `
		{
			friend(func: uid(1000)) {
				path @facets(L1 as weight) {
						path @facets(L2 as weight) {
							c as count(follow)
							L4 as math(c+L2+L1)
						}
				}
			}

			sum(func: uid(L4), orderdesc: val(L4)) {
				name
				val(L4)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"friend":[{"path":[{"path":[{"count(follow)":1,"val(L4)":1.200000,"path|weight":0.100000},{"count(follow)":1,"val(L4)":3.900000,"path|weight":1.500000}],"path|weight":0.100000},{"path":[{"count(follow)":1,"val(L4)":3.900000,"path|weight":0.600000}],"path|weight":0.700000}]}],"sum":[{"name":"John","val(L4)":3.900000},{"name":"Matt","val(L4)":1.200000}]}}`,
		js)
}

func TestLevelBasedSumMix1(t *testing.T) {
	query := `
		{
			friend(func: uid( 1)) {
				a as age
				path @facets(L1 as weight) {
					L2 as math(a+L1)
			 	}
			}
			sum(func: uid(L2), orderdesc: val(L2)) {
				name
				val(L2)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friend":[{"age":38,"path":[{"val(L2)":38.200000,"path|weight":0.200000},{"val(L2)":38.100000,"path|weight":0.100000}]}],"sum":[{"name":"Glenn Rhee","val(L2)":38.200000},{"name":"Andrea","val(L2)":38.100000}]}}`,
		js)
}

func TestLevelBasedFacetVarSum1(t *testing.T) {
	query := `
		{
			friend(func: uid( 1000)) {
				path @facets(L1 as weight) {
					name
					path @facets(L2 as weight) {
						L3 as math(L1+L2)
					}
			 }
			}
			sum(func: uid(L3), orderdesc: val(L3)) {
				name
				val(L3)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friend":[{"path":[{"name":"Bob","path":[{"val(L3)":0.200000,"path|weight":0.100000},{"val(L3)":2.900000,"path|weight":1.500000}],"path|weight":0.100000},{"name":"Matt","path":[{"val(L3)":2.900000,"path|weight":0.600000}],"path|weight":0.700000}]}],"sum":[{"name":"John","val(L3)":2.900000},{"name":"Matt","val(L3)":0.200000}]}}`,
		js)
}

func TestLevelBasedFacetVarSum2(t *testing.T) {
	query := `
		{
			friend(func: uid( 1000)) {
				path @facets(L1 as weight) {
					path @facets(L2 as weight) {
						path @facets(L3 as weight) {
							L4 as math(L1+L2+L3)
						}
					}
				}
			}
			sum(func: uid(L4), orderdesc: val(L4)) {
				name
				val(L4)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friend":[{"path":[{"path":[{"path":[{"val(L4)":0.800000,"path|weight":0.600000}],"path|weight":0.100000},{"path":[{"val(L4)":2.900000}],"path|weight":1.500000}],"path|weight":0.100000},{"path":[{"path":[{"val(L4)":2.900000}],"path|weight":0.600000}],"path|weight":0.700000}]}],"sum":[{"name":"Bob","val(L4)":2.900000},{"name":"John","val(L4)":0.800000}]}}`,
		js)
}

func TestQueryConstMathVal(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as math(24/8 * 3)
			}

			AgeOrder(func: uid(f)) {
				name
				val(a)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"AgeOrder":[{"name":"Michonne","val(a)":9.000000},{"name":"Rick Grimes","val(a)":9.000000},{"name":"Andrea","val(a)":9.000000},{"name":"Andrea With no friends","val(a)":9.000000}]}}`,
		js)
}

func TestQueryVarValAggSince(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as dob
				b as math(since(a)/(60*60*24*365))
			}

			AgeOrder(func: uid(f), orderasc: val(b)) {
				name
				val(a)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"AgeOrder":[{"name":"Rick Grimes","val(a)":"1910-01-02T00:00:00Z"},{"name":"Michonne","val(a)":"1910-01-01T00:00:00Z"},{"name":"Andrea","val(a)":"1901-01-15T00:00:00Z"}]}}`,
		js)
}

func TestQueryVarValAggNestedFuncConst(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				p as math(a + s % n + 10)
				q as math(a * s * n * -1)
			}

			MaxMe(func: uid(f), orderasc: val(p)) {
				name
				val(p)
				val(a)
				val(n)
				val(s)
			}

			MinMe(func: uid(f), orderasc: val(q)) {
				name
				val(q)
				val(a)
				val(n)
				val(s)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"MaxMe":[{"name":"Rick Grimes","val(a)":15,"val(n)":38,"val(p)":25.000000,"val(s)":38},{"name":"Andrea","val(a)":19,"val(n)":15,"val(p)":29.000000,"val(s)":15},{"name":"Michonne","val(a)":38,"val(n)":15,"val(p)":52.000000,"val(s)":19}],"MinMe":[{"name":"Rick Grimes","val(a)":15,"val(n)":38,"val(q)":-21660.000000,"val(s)":38},{"name":"Michonne","val(a)":38,"val(n)":15,"val(q)":-10830.000000,"val(s)":19},{"name":"Andrea","val(a)":19,"val(n)":15,"val(q)":-4275.000000,"val(s)":15}]}}`,
		js)
}

func TestQueryVarValAggNestedFuncMinMaxVars(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				p as math(max(max(a, s), n))
				q as math(min(min(a, s), n))
			}

			MaxMe(func: uid(f), orderasc: val(p)) {
				name
				val(p)
				val(a)
				val(n)
				val(s)
			}

			MinMe(func: uid(f), orderasc: val(q)) {
				name
				val(q)
				val(a)
				val(n)
				val(s)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"MinMe":[{"name":"Michonne","val(a)":38,"val(n)":15,"val(q)":15,"val(s)":19},{"name":"Rick Grimes","val(a)":15,"val(n)":38,"val(q)":15,"val(s)":38},{"name":"Andrea","val(a)":19,"val(n)":15,"val(q)":15,"val(s)":15}],"MaxMe":[{"name":"Andrea","val(a)":19,"val(n)":15,"val(p)":19,"val(s)":15},{"name":"Michonne","val(a)":38,"val(n)":15,"val(p)":38,"val(s)":19},{"name":"Rick Grimes","val(a)":15,"val(n)":38,"val(p)":38,"val(s)":38}]}}`,
		js)
}

func TestQueryVarValAggNestedFuncConditional(t *testing.T) {
	query := `
	{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				condLog as math(cond(a > 10, logbase(n, 5), 1))
				condExp as math(cond(a < 40, 1, pow(2, n)))
			}

			LogMe(func: uid(f), orderasc: val(condLog)) {
				name
				val(condLog)
				val(n)
				val(a)
			}

			ExpMe(func: uid(f), orderasc: val(condExp)) {
				name
				val(condExp)
				val(n)
				val(a)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"ExpMe":[{"name":"Michonne","val(a)":38,"val(condExp)":1.000000,"val(n)":15},{"name":"Rick Grimes","val(a)":15,"val(condExp)":1.000000,"val(n)":38},{"name":"Andrea","val(a)":19,"val(condExp)":1.000000,"val(n)":15}],"LogMe":[{"name":"Michonne","val(a)":38,"val(condLog)":1.682606,"val(n)":15},{"name":"Andrea","val(a)":19,"val(condLog)":1.682606,"val(n)":15},{"name":"Rick Grimes","val(a)":15,"val(condLog)":2.260159,"val(n)":38}]}}`,
		js)
}

func TestQueryVarValAggNestedFuncConditional2(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				condLog as math(cond(a==38, n/2, 1))
				condExp as math(cond(a!=38, 1, sqrt(2*n)))
			}

			LogMe(func: uid(f), orderasc: val(condLog)) {
				name
				val(condLog)
				val(n)
				val(a)
			}

			ExpMe(func: uid(f), orderasc: val(condExp)) {
				name
				val(condExp)
				val(n)
				val(a)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"ExpMe":[{"name":"Rick Grimes","val(a)":15,"val(condExp)":1.000000,"val(n)":38},{"name":"Andrea","val(a)":19,"val(condExp)":1.000000,"val(n)":15},{"name":"Michonne","val(a)":38,"val(condExp)":5.477226,"val(n)":15}],"LogMe":[{"name":"Rick Grimes","val(a)":15,"val(condLog)":1.000000,"val(n)":38},{"name":"Andrea","val(a)":19,"val(condLog)":1.000000,"val(n)":15},{"name":"Michonne","val(a)":38,"val(condLog)":7.500000,"val(n)":15}]}}`,
		js)
}

func TestQueryVarValAggNestedFuncUnary(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				combiLog as math(a + ln(s - n))
				combiExp as math(a + exp(s - n))
			}

			LogMe(func: uid(f), orderasc: val(combiLog)) {
				name
				val(combiLog)
				val(a)
				val(n)
				val(s)
			}

			ExpMe(func: uid(f), orderasc: val(combiExp)) {
				name
				val(combiExp)
				val(a)
				val(n)
				val(s)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"ExpMe":[{"name":"Rick Grimes","val(a)":15,"val(combiExp)":16.000000,"val(n)":38,"val(s)":38},{"name":"Andrea","val(a)":19,"val(combiExp)":20.000000,"val(n)":15,"val(s)":15},{"name":"Michonne","val(a)":38,"val(combiExp)":92.598150,"val(n)":15,"val(s)":19}],"LogMe":[{"name":"Rick Grimes","val(a)":15,"val(combiLog)":-179769313486231570814527423731704356798070567525844996598917476803157260780028538760589558632766878171540458953514382464234321326889464182768467546703537516986049910576551282076245490090389328944075868508455133942304583236903222948165808559332123348274797826204144723168738177180919299881250404026184124858368.000000,"val(n)":38,"val(s)":38},{"name":"Andrea","val(a)":19,"val(combiLog)":-179769313486231570814527423731704356798070567525844996598917476803157260780028538760589558632766878171540458953514382464234321326889464182768467546703537516986049910576551282076245490090389328944075868508455133942304583236903222948165808559332123348274797826204144723168738177180919299881250404026184124858368.000000,"val(n)":15,"val(s)":15},{"name":"Michonne","val(a)":38,"val(combiLog)":39.386294,"val(n)":15,"val(s)":19}]}}`,
		js)
}

func TestQueryVarValAggNestedFunc(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				combi as math(a + n * s)
			}

			me(func: uid(f), orderasc: val(combi)) {
				name
				val(combi)
				val(a)
				val(n)
				val(s)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea","val(a)":19,"val(combi)":244,"val(n)":15,"val(s)":15},{"name":"Michonne","val(a)":38,"val(combi)":323,"val(n)":15,"val(s)":19},{"name":"Rick Grimes","val(a)":15,"val(combi)":1459,"val(n)":38,"val(s)":38}]}}`,
		js)
}

func TestQueryVarValAggMinMaxSelf(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				sum as math(n +  a + s)
			}

			me(func: uid(f), orderasc: val(sum)) {
				name
				val(sum)
				val(s)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea","val(s)":15,"val(sum)":49},{"name":"Michonne","val(s)":19,"val(sum)":72},{"name":"Rick Grimes","val(s)":38,"val(sum)":91}]}}`,
		js)
}

func TestQueryVarValAggMinMax(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				sum as math(n + s)
			}

			me(func: uid(f), orderdesc: val(sum)) {
				name
				val(n)
				val(s)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Rick Grimes","val(n)":38,"val(s)":38},{"name":"Michonne","val(n)":15,"val(s)":19},{"name":"Andrea","val(n)":15,"val(s)":15}]}}`,
		js)
}

func TestQueryVarValAggMinMaxAlias(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				sum as math(n + s)
			}

			me(func: uid(f), orderdesc: val(sum)) {
				name
				MinAge: val(n)
				MaxAge: val(s)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Rick Grimes","MinAge":38,"MaxAge":38},{"name":"Michonne","MinAge":15,"MaxAge":19},{"name":"Andrea","MinAge":15,"MaxAge":15}]}}`,
		js)
}

func TestQueryVarValAggMul(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				f as friend {
					n as age
					s as count(friend)
					mul as math(n * s)
				}
			}

			me(func: uid(f), orderdesc: val(mul)) {
				name
				val(s)
				val(n)
				val(mul)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea","val(mul)":19.000000,"val(n)":19,"val(s)":1},{"name":"Rick Grimes","val(mul)":15.000000,"val(n)":15,"val(s)":1},{"name":"Glenn Rhee","val(mul)":0.000000,"val(n)":15,"val(s)":0},{"name":"Daryl Dixon","val(mul)":0.000000,"val(n)":17,"val(s)":0},{"val(mul)":0.000000,"val(s)":0}]}}`,
		js)
}

func TestQueryVarValAggOrderDesc(t *testing.T) {
	query := `
		{
			info(func: uid( 1)) {
				f as friend {
					n as age
					s as count(friend)
					sum as math(n + s)
				}
			}

			me(func: uid(f), orderdesc: val(sum)) {
				name
				age
				count(friend)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"info":[{"friend":[{"age":15,"count(friend)":1,"val(sum)":16.000000},{"age":15,"count(friend)":0,"val(sum)":15.000000},{"age":17,"count(friend)":0,"val(sum)":17.000000},{"age":19,"count(friend)":1,"val(sum)":20.000000},{"count(friend)":0,"val(sum)":0.000000}]}],"me":[{"age":19,"count(friend)":1,"name":"Andrea"},{"age":17,"count(friend)":0,"name":"Daryl Dixon"},{"age":15,"count(friend)":1,"name":"Rick Grimes"},{"age":15,"count(friend)":0,"name":"Glenn Rhee"},{"count(friend)":0}]}}`,
		js)
}

func TestQueryVarValAggOrderAsc(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				f as friend {
					n as age
					s as survival_rate
					sum as math(n + s)
				}
			}

			me(func: uid(f), orderasc: val(sum)) {
				name
				age
				survival_rate
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"age":15,"name":"Rick Grimes","survival_rate":1.600000},{"age":15,"name":"Glenn Rhee","survival_rate":1.600000},{"age":17,"name":"Daryl Dixon","survival_rate":1.600000},{"age":19,"name":"Andrea","survival_rate":1.600000}]}}`,
		js)
}

func TestQueryVarValOrderAsc(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				f as friend {
					n as name
				}
			}

			me(func: uid(f), orderasc: val(n)) {
				name
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea"},{"name":"Daryl Dixon"},{"name":"Glenn Rhee"},{"name":"Rick Grimes"}]}}`,
		js)
}

func TestQueryVarValOrderDob(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				f as friend {
					n as dob
				}
			}

			me(func: uid(f), orderasc: val(n)) {
				name
				dob
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea", "dob":"1901-01-15T00:00:00Z"},{"name":"Daryl Dixon", "dob":"1909-01-10T00:00:00Z"},{"name":"Glenn Rhee", "dob":"1909-05-05T00:00:00Z"},{"name":"Rick Grimes", "dob":"1910-01-02T00:00:00Z"}]}}`,
		js)
}

func TestQueryVarValOrderError(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				friend {
					n as name
				}
			}

			me(func: uid(n), orderdesc: n) {
				name
			}
		}
	`
	_, err := processToFastJson(t, query)
	require.Contains(t, err.Error(), "Cannot sort attribute n of type object.")
}

func TestQueryVarValOrderDesc(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				f as friend {
					n as name
				}
			}

			me(func: uid(f), orderdesc: val(n)) {
				name
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}}`,
		js)
}

func TestQueryVarValOrderDescMissing(t *testing.T) {
	query := `
		{
			var(func: uid( 1034)) {
				f As friend {
					n As name
				}
			}

			me(func: uid(f), orderdesc: val(n)) {
				name
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestGroupByRoot(t *testing.T) {
	query := `
	{
		me(func: uid(1, 23, 24, 25, 31)) @groupby(age) {
				count(uid)
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"@groupby":[{"age":17,"count":1},{"age":19,"count":1},{"age":38,"count":1},{"age":15,"count":2}]}]}}`,
		js)
}

func TestGroupByRootEmpty(t *testing.T) {
	// Predicate agent doesn't exist.
	query := `
	{
		me(func: uid(1, 23, 24, 25, 31)) @groupby(agent) {
				count(uid)
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {}}`, js)
}

func TestGroupByRootAlias(t *testing.T) {
	query := `
	{
		me(func: uid(1, 23, 24, 25, 31)) @groupby(age) {
			Count: count(uid)
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"@groupby":[{"age":17,"Count":1},{"age":19,"Count":1},{"age":38,"Count":1},{"age":15,"Count":2}]}]}}`, js)
}

func TestGroupByRootAlias2(t *testing.T) {
	query := `
	{
		me(func: uid(1, 23, 24, 25, 31)) @groupby(Age: age) {
			Count: count(uid)
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"@groupby":[{"Age":17,"Count":1},{"Age":19,"Count":1},{"Age":38,"Count":1},{"Age":15,"Count":2}]}]}}`, js)
}

func TestGroupBy_RepeatAttr(t *testing.T) {
	query := `
	{
		me(func: uid(1)) {
			friend @groupby(age) {
				count(uid)
			}
			friend {
				name
				age
			}
			name
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"@groupby":[{"age":17,"count":1},{"age":19,"count":1},{"age":15,"count":2}]},{"age":15,"name":"Rick Grimes"},{"age":15,"name":"Glenn Rhee"},{"age":17,"name":"Daryl Dixon"},{"age":19,"name":"Andrea"}],"name":"Michonne"}]}}`,
		js)
}

func TestGroupBy(t *testing.T) {
	query := `
	{
		age(func: uid(1)) {
			friend {
				age
				name
			}
		}

		me(func: uid(1)) {
			friend @groupby(age) {
				count(uid)
			}
			name
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"age":[{"friend":[{"age":15,"name":"Rick Grimes"},{"age":15,"name":"Glenn Rhee"},{"age":17,"name":"Daryl Dixon"},{"age":19,"name":"Andrea"}]}],"me":[{"friend":[{"@groupby":[{"age":17,"count":1},{"age":19,"count":1},{"age":15,"count":2}]}],"name":"Michonne"}]}}`,
		js)
}

func TestGroupByCountval(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				friend @groupby(school) {
					a as count(uid)
				}
			}

			order(func :uid(a), orderdesc: val(a)) {
				name
				val(a)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"order":[{"name":"School B","val(a)":3},{"name":"School A","val(a)":2}]}}`,
		js)
}
func TestGroupByAggval(t *testing.T) {
	query := `
		{
			var(func: uid(1)) {
				friend @groupby(school) {
					a as max(name)
					b as min(name)
				}
			}

			orderMax(func :uid(a), orderdesc: val(a)) {
				name
				val(a)
			}

			orderMin(func :uid(b), orderdesc: val(b)) {
				name
				val(b)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"orderMax":[{"name":"School B","val(a)":"Rick Grimes"},{"name":"School A","val(a)":"Glenn Rhee"}],"orderMin":[{"name":"School A","val(b)":"Daryl Dixon"},{"name":"School B","val(b)":"Andrea"}]}}`,
		js)
}

func TestGroupByAlias(t *testing.T) {
	query := `
		{
			me(func: uid(1)) {
				friend @groupby(school) {
					MaxName: max(name)
					MinName: min(name)
					UidCount: count(uid)
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"friend":[{"@groupby":[{"school":"0x1388","MaxName":"Glenn Rhee","MinName":"Daryl Dixon","UidCount":2},{"school":"0x1389","MaxName":"Rick Grimes","MinName":"Andrea","UidCount":3}]}]}]}}`, js)
}

func TestGroupByAgg(t *testing.T) {
	query := `
		{
			me(func: uid( 1)) {
				friend @groupby(age) {
					max(name)
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"@groupby":[{"age":17,"max(name)":"Daryl Dixon"},{"age":19,"max(name)":"Andrea"},{"age":15,"max(name)":"Rick Grimes"}]}]}]}}`,
		js)
}

func TestGroupByMulti(t *testing.T) {
	query := `
		{
			me(func: uid(1)) {
				friend @groupby(FRIEND: friend,name) {
					count(uid)
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"@groupby":[{"count":1,"FRIEND":"0x1","name":"Rick Grimes"},{"count":1,"FRIEND":"0x18","name":"Andrea"}]}]}]}}`,
		js)
}

func TestGroupByMulti2(t *testing.T) {
	query := `
		{
			me(func: uid(1)) {
				Friend: friend @groupby(Friend: friend,Name: name) {
					Count: count(uid)
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"Friend":[{"@groupby":[{"Friend":"0x1","Name":"Rick Grimes","Count":1},{"Friend":"0x18","Name":"Andrea","Count":1}]}]}]}}`,
		js)
}

func TestGroupByMultiParents(t *testing.T) {
	query := `
		{
			me(func: uid(1,23,31)) {
				name
				friend @groupby(name, age) {
					count(uid)
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"name":"Michonne","friend":[{"@groupby":[{"name":"Andrea","age":19,"count":1},{"name":"Daryl Dixon","age":17,"count":1},{"name":"Glenn Rhee","age":15,"count":1},{"name":"Rick Grimes","age":15,"count":1}]}]},{"name":"Rick Grimes","friend":[{"@groupby":[{"name":"Michonne","age":38,"count":1}]}]},{"name":"Andrea","friend":[{"@groupby":[{"name":"Glenn Rhee","age":15,"count":1}]}]}]}}`, js)
}

func TestGroupByMultiParents_2(t *testing.T) {
	// We dont have any data for uid 99999
	query := `
		{
			me(func: uid(1,23,99999,31)) {
				name
				friend @groupby(name, age) {
					count(uid)
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"name":"Michonne","friend":[{"@groupby":[{"name":"Andrea","age":19,"count":1},{"name":"Daryl Dixon","age":17,"count":1},{"name":"Glenn Rhee","age":15,"count":1},{"name":"Rick Grimes","age":15,"count":1}]}]},{"name":"Rick Grimes","friend":[{"@groupby":[{"name":"Michonne","age":38,"count":1}]}]},{"name":"Andrea","friend":[{"@groupby":[{"name":"Glenn Rhee","age":15,"count":1}]}]}]}}`, js)

}

func TestGroupByAgeMultiParents(t *testing.T) {
	// We dont have any data for uid 99999, 99998.
	query := `
		{
			me(func: uid(23,99999,31, 99998,1)) {
				name
				friend @groupby(age) {
					count(uid)
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"name":"Michonne","friend":[{"@groupby":[{"age":17,"count":1},{"age":19,"count":1},{"age":15,"count":2}]}]},{"name":"Rick Grimes","friend":[{"@groupby":[{"age":38,"count":1}]}]},{"name":"Andrea","friend":[{"@groupby":[{"age":15,"count":1}]}]}]}}`, js)
}

func TestGroupByFriendsMultipleParents(t *testing.T) {

	// We dont have any data for uid 99999, 99998.
	query := `
		{
			me(func: uid(23,99999,31, 99998,1)) {
				name
				friend @groupby(friend) {
					count(uid)
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"name":"Michonne","friend":[{"@groupby":[{"friend":"0x1","count":1},{"friend":"0x18","count":1}]}]},{"name":"Rick Grimes","friend":[{"@groupby":[{"friend":"0x17","count":1},{"friend":"0x18","count":1},{"friend":"0x19","count":1},{"friend":"0x1f","count":1},{"friend":"0x65","count":1}]}]},{"name":"Andrea"}]}}`, js)
}

func TestGroupByFriendsMultipleParentsVar(t *testing.T) {

	// We dont have any data for uid 99999, 99998.
	query := `
		{
			var(func: uid(23,99999,31, 99998,1)) {
				name
				friend @groupby(friend) {
					f as count(uid)
				}
			}

			me(func: uid(f), orderdesc: val(f)) {
				uid
				name
				val(f)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"uid":"0x18","name":"Glenn Rhee","val(f)":2},{"uid":"0x1","name":"Michonne","val(f)":1},{"uid":"0x17","name":"Rick Grimes","val(f)":1},{"uid":"0x19","name":"Daryl Dixon","val(f)":1},{"uid":"0x1f","name":"Andrea","val(f)":1},{"uid":"0x65","val(f)":1}]}}`, js)
}

func TestMultiEmptyBlocks(t *testing.T) {

	query := `
		{
			you(func: uid(0x01)) {
			}

			me(func: uid(0x02)) {
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"you": [], "me": []}}`, js)
}

func TestUseVarsMultiCascade1(t *testing.T) {

	query := `
		{
			him(func: uid(0x01)) @cascade {
				L as friend {
					B as friend
					name
			 	}
			}

			me(func: uid(L, B)) {
				name
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"him": [{"friend":[{"name":"Rick Grimes"}, {"name":"Andrea"}]}], "me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}, {"name":"Andrea"}]}}`,
		js)
}

func TestUseVarsMultiCascade(t *testing.T) {

	query := `
		{
			var(func: uid(0x01)) @cascade {
				L as friend {
				 	B as friend
				}
			}

			me(func: uid(L, B)) {
				name
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}, {"name":"Andrea"}]}}`,
		js)
}

func TestUseVarsMultiOrder(t *testing.T) {

	query := `
		{
			var(func: uid(0x01)) {
				L as friend(first:2, orderasc: dob)
			}

			var(func: uid(0x01)) {
				G as friend(first:2, offset:2, orderasc: dob)
			}

			friend1(func: uid(L)) {
				name
			}

			friend2(func: uid(G)) {
				name
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"friend1":[{"name":"Daryl Dixon"}, {"name":"Andrea"}],"friend2":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}}`,
		js)
}

func TestFilterFacetval(t *testing.T) {

	query := `
		{
			friend(func: uid(0x01)) {
				path @facets(L as weight) {
					name
				 	friend @filter(uid(L)) {
						name
						val(L)
					}
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friend":[{"path":[{"name":"Glenn Rhee","path|weight":0.200000},{"name":"Andrea","friend":[{"name":"Glenn Rhee","val(L)":0.200000}],"path|weight":0.100000}]}]}}`,
		js)
}

func TestFilterFacetVar1(t *testing.T) {

	query := `
		{
			friend(func: uid(0x01)) {
				path @facets(L as weight1) {
					name
				 	friend @filter(uid(L)){
						name
					}
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friend":[{"path":[{"name":"Glenn Rhee"},{"name":"Andrea","path|weight1":0.200000}]}]}}`,
		js)
}

func TestUseVarsFilterVarReuse1(t *testing.T) {

	query := `
		{
			friend(func: uid(0x01)) {
				friend {
					L as friend {
						name
						friend @filter(uid(L)) {
							name
						}
					}
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"friend":[{"friend":[{"friend":[{"name":"Michonne", "friend":[{"name":"Glenn Rhee"}]}]}, {"friend":[{"name":"Glenn Rhee"}]}]}]}}`,
		js)
}

func TestUseVarsFilterVarReuse2(t *testing.T) {

	query := `
		{
			friend(func:anyofterms(name, "Michonne Andrea Glenn")) {
				friend {
				 L as friend {
					nonexistent_pred
					name
					friend @filter(uid(L)) {
						name
					}
				}
			}
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"friend":[{"friend":[{"friend":[{"name":"Michonne", "friend":[{"name":"Glenn Rhee"}]}]}, {"friend":[{"name":"Glenn Rhee"}]}]}]}}`,
		js)
}

func TestDoubleOrder(t *testing.T) {

	query := `
    {
		me(func: uid(1)) {
			friend(orderdesc: dob) @facets(orderasc: weight)
		}
	}
  `
	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestVarInAggError(t *testing.T) {

	query := `
    {
			var(func: uid( 1)) {
				friend {
					a as age
				}
			}

			# var not allowed in min filter
			me(func: min(val(a))) {
				name
			}
		}
  `
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Function name: min is not valid.")
}

func TestVarInIneqError(t *testing.T) {

	query := `
    {
			var(func: uid( 1)) {
				f as friend {
					a as age
				}
			}

			me(func: uid(f)) @filter(gt(val(a), "alice")) {
				name
			}
		}
  `
	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestVarInIneqScore(t *testing.T) {

	query := `
    {
			var(func: uid( 1)) {
				friend {
					a as age
					s as count(friend)
					score as math(2*a + 3 * s + 1)
				}
			}

			me(func: ge(val(score), 35)) {
				name
				val(score)
				val(a)
				val(s)
			}
		}
  `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Daryl Dixon","val(a)":17,"val(s)":0,"val(score)":35.000000},{"name":"Andrea","val(a)":19,"val(s)":1,"val(score)":42.000000}]}}`,
		js)
}

func TestVarInIneq(t *testing.T) {

	query := `
    {
			var(func: uid( 1)) {
				f as friend {
					a as age
				}
			}

			me(func: uid(f)) @filter(gt(val(a), 18)) {
				name
			}
		}
  `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Andrea"}]}}`, js)
}

func TestVarInIneq2(t *testing.T) {

	query := `
    {
			var(func: uid(1)) {
				friend {
					a as age
				}
			}

			me(func: gt(val(a), 18)) {
				name
			}
		}
  `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Andrea"}]}}`, js)
}

func TestVarInIneq3(t *testing.T) {

	query := `
    {
			var(func: uid(0x1f)) {
				a as name
			}

			me(func: eq(name, val(a))) {
				name
			}
		}
  `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Andrea"}]}}`, js)
}

func TestVarInIneq4(t *testing.T) {

	query := `
    {
			var(func: uid(0x1f)) {
				a as name
			}

			me(func: uid(0x1f)) @filter(eq(name, val(a))) {
				name
			}
		}
  `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Andrea"}]}}`, js)
}

func TestVarInIneq5(t *testing.T) {

	query1 := `
    {
			var(func: uid(1)) {
				friend {
				  a as name
			  }
			}

			me(func: eq(name, val(a))) {
				name
			}
		}
  `
	query2 := `
    {
			var(func: uid(1)) {
				friend {
				  a as name
			  }
			}

			me(func: uid(a)) {
				name: val(a)
			}
		}
  `
	js1 := processToFastJsonNoErr(t, query1)
	js2 := processToFastJsonNoErr(t, query2)
	require.JSONEq(t, js2, js1)
}

func TestNestedFuncRoot(t *testing.T) {

	query := `
    {
			me(func: gt(count(friend), 2)) {
				name
			}
		}
  `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"}]}}`, js)
}

func TestNestedFuncRoot2(t *testing.T) {

	query := `
		{
			me(func: ge(count(friend), 1)) {
				name
			}
		}
  `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Andrea"}]}}`, js)
}

func TestNestedFuncRoot4(t *testing.T) {

	query := `
		{
			me(func: le(count(friend), 1)) {
				name
			}
		}
  `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"},{"name":"Andrea"}]}}`, js)
}

var maxPendingCh chan uint64

func TestMain(m *testing.M) {
	x.Init(true)

	odch = make(chan *intern.OracleDelta, 100)
	maxPendingCh = make(chan uint64, 100)

	cmd := exec.Command("go", "install", "github.com/dgraph-io/dgraph/dgraph")
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("Could not run %q: %s", cmd.Args, string(out))
	}
	zw, err := ioutil.TempDir("", "wal_")
	x.Check(err)

	zero := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"zero",
		"--wal", zw,
		"-o", "10",
	)
	zero.Stdout = os.Stdout
	zero.Stderr = os.Stdout
	if err := zero.Start(); err != nil {
		log.Fatalf("While starting Zero: %v", err)
	}

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)

	opt := badger.LSMOnlyOptions
	opt.Dir = dir
	opt.ValueDir = dir
	opt.SyncWrites = false
	ps, err = badger.OpenManaged(opt)
	defer ps.Close()
	x.Check(err)

	worker.Config.RaftId = 1
	posting.Config.AllottedMemory = 1024.0
	posting.Config.CommitFraction = 0.10
	worker.Config.ZeroAddr = fmt.Sprintf("localhost:%d", x.PortZeroGrpc+10)
	worker.Config.RaftId = 1
	worker.Config.MyAddr = "localhost:12345"
	worker.Config.ExpandEdge = true
	worker.Config.NumPendingProposals = 100 // So that mutations can run.
	schema.Init(ps)
	posting.Init(ps)
	worker.Init(ps)

	dir2, err := ioutil.TempDir("", "wal_")
	x.Check(err)

	kvOpt := badger.DefaultOptions
	kvOpt.SyncWrites = true
	kvOpt.Dir = dir2
	kvOpt.ValueDir = dir2
	kvOpt.TableLoadingMode = options.LoadToRAM
	walStore, err := badger.Open(kvOpt)
	x.Check(err)

	worker.StartRaftNodes(walStore, false)
	// Load schema after nodes have started
	err = schema.ParseBytes([]byte(schemaStr), 1)
	x.Check(err)

	populateGraph(&testing.T{})
	go updateMaxPending()
	r := m.Run()

	os.RemoveAll(dir)
	os.RemoveAll(dir2)
	x.Check(zero.Process.Kill())
	os.RemoveAll(zw)
	os.Exit(r)
}
