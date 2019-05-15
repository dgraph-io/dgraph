// +build standalone

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
	"flag"
	"io/ioutil"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"

	"github.com/stretchr/testify/require"
)

// JSON output can be hundreds of lines and diffs can scroll off the terminal before you
// can look at them. This option allows saving the JSON to a specified directory instead
// for easier reviewing after the test completes.
var savedir = flag.String("savedir", "",
	"directory to save json from test failures in")
var quiet = flag.Bool("quiet", false,
	"just output whether json differs, not a diff")

func TestQueries(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	queryDir := path.Join(path.Dir(thisFile), "queries")

	// For this test we DON'T want to start with an empty database.
	dg := z.DgraphClient(z.SockAddr)

	files, err := ioutil.ReadDir(queryDir)
	x.CheckfNoTrace(err)

	savepath := ""
	diffs := 0
	for _, file := range files {
		if !strings.HasPrefix(file.Name(), "query-") {
			continue
		}

		filename := path.Join(queryDir, file.Name())
		reader, cleanup := chunker.FileReader(filename)
		bytes, err := ioutil.ReadAll(reader)
		x.CheckfNoTrace(err)
		contents := string(bytes[:])
		cleanup()

		// The test query and expected result are separated by a delimiter.
		bodies := strings.SplitN(contents, "\n---\n", 2)

		// If a query takes too long to run, it probably means dgraph is stuck and there's
		// no point in waiting longer or trying more tests.
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		resp, err := dg.NewTxn().Query(ctx, bodies[0])
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatal("aborting test due to query timeout")
		}
		require.NoError(t, err)

		t.Logf("running %s", file.Name())
		if *savedir != "" {
			savepath = path.Join(*savedir, file.Name())
		}

		if !z.EqualJSON(t, bodies[1], string(resp.GetJson()), savepath, *quiet) {
			diffs++
		}
	}

	if *savedir != "" && diffs > 0 {
		t.Logf("test json saved in directory: %s", *savedir)
	}
}
