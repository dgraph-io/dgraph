/*
* Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package main

import (
    "bytes"
    // "fmt"
    // "os"
)

// pred, uid, ubytes, pbytes
func concatenatePostings(x, y interface{}) (interface{}, error) {
	xx := x.([]interface{})
	yy := y.([]interface{})

    xuid := xx[1].([]byte)
    yuid := yy[1].([]byte)

    // check if this is a duplicate entry (UID match)
    if bytes.Equal(xuid, yuid) {
        return xx, nil
    } else {
        return []interface{}{
            yy[0],
            yuid,
            append(xx[2].([]byte), yy[2].([]byte)...),
            append(xx[3].([]byte), yy[3].([]byte)...),
        }, nil
    }
}
