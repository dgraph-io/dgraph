/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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
	"context"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
	"io/ioutil"
	"path"
	"runtime"
	"strings"
)

func main() {
	_, thisFile, _, _ := runtime.Caller(0)
	queryDir := path.Dir(thisFile) + "/query"

	dg := z.DgraphClient(":9180")

	files, err := ioutil.ReadDir(queryDir)
	x.CheckfNoTrace(err)

	for _, file := range files {
		contents, err := ioutil.ReadFile(queryDir + "/" + file.Name())
		x.CheckfNoTrace(err)
		content := strings.SplitN(string(contents), "\n---\n", 2)

		resp, err := dg.NewTxn().Query(context.Background(), content[0])
		x.Checkf(err, "on file %s with query %s:", file, content[0])

		z.CompareJSON(nil, `
		{
		    "q": [
					{
					"name": "Homer",
					"age": 38,
					"role": "father"
			    }
			]
		}
	`, string(resp.GetJson()))
	}
}
