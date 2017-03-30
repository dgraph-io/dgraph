/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package client

import (
	"fmt"
	"time"

	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/types"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

func ValueFromGeoJson(json string, nq *graphp.NQuad) error {
	var g geom.T
	// Parse the json
	err := geojson.Unmarshal([]byte(json), &g)
	if err != nil {
		return err
	}

	geo, err := types.ObjectValue(types.GeoID, g)
	if err != nil {
		return err
	}

	nq.ObjectValue = geo
	nq.ObjectType = int32(types.GeoID)
	return nil
}

func Date(date time.Time, nq *graphp.NQuad) error {
	d, err := types.ObjectValue(types.DateID, date)
	if err != nil {
		return err
	}
	nq.ObjectValue = d
	nq.ObjectType = int32(types.DateID)
	return nil
}

func Datetime(date time.Time, nq *graphp.NQuad) error {
	d, err := types.ObjectValue(types.DateTimeID, date)
	if err != nil {
		return err
	}
	nq.ObjectValue = d
	nq.ObjectType = int32(types.DateTimeID)
	return nil
}

func Str(val string, nq *graphp.NQuad) error {
	v, err := types.ObjectValue(types.StringID, val)
	if err != nil {
		return err
	}
	nq.ObjectValue = v
	nq.ObjectType = int32(types.StringID)
	return nil
}

func Int(val int64, nq *graphp.NQuad) error {
	v, err := types.ObjectValue(types.IntID, val)
	if err != nil {
		return err
	}
	nq.ObjectValue = v
	nq.ObjectType = int32(types.IntID)
	return nil

}

func Float(val float64, nq *graphp.NQuad) error {
	v, err := types.ObjectValue(types.FloatID, val)
	if err != nil {
		return err
	}
	nq.ObjectValue = v
	nq.ObjectType = int32(types.FloatID)
	return nil

}

func Bool(val bool, nq *graphp.NQuad) error {
	v, err := types.ObjectValue(types.BoolID, val)
	if err != nil {
		return err
	}
	nq.ObjectValue = v
	nq.ObjectType = int32(types.BoolID)
	return nil
}

// Uid converts an uint64 to a string, which can be used as part of
// Subject and ObjectId fields in the graphp.NQuad
func Uid(uid uint64) string {
	return fmt.Sprintf("%#x", uid)
}

func Password(val string, nq *graphp.NQuad) error {
	v, err := types.ObjectValue(types.PasswordID, val)
	if err != nil {
		return err
	}
	nq.ObjectValue = v
	nq.ObjectType = int32(types.PasswordID)
	return nil
}
