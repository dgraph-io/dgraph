/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

var BaseUrl = "https://github.com/dgraph-io/benchmarks/blob/master/ldbc/sf0.3/ldbc_rdf_0.3/"
var Suffix = "?raw=true"

var ldbcDataFiles = map[string]string{
	"ldbcTypes.schema": "https://github.com/dgraph-io/benchmarks/blob/master/ldbc/sf0.3/ldbcTypes.schema?raw=true",
}

var rdfFileNames = [...]string{
	"Deltas.rdf",
	"comment_0.rdf",
	"containerOf_0.rdf",
	"forum_0.rdf",
	"hasCreator_0.rdf",
	"hasInterest_0.rdf",
	"hasMember_0.rdf",
	"hasModerator_0.rdf",
	"hasTag_0.rdf",
	"hasType_0.rdf",
	"isLocatedIn_0.rdf",
	"isPartOf_0.rdf",
	"isSubclassOf_0.rdf",
	"knows_0.rdf",
	"likes_0.rdf",
	"organisation_0.rdf",
	"person_0.rdf",
	"place_0.rdf",
	"post_0.rdf",
	"replyOf_0.rdf",
	"studyAt_0.rdf",
	"tag_0.rdf",
	"tagclass_0.rdf",
	"workAt_0.rdf",
}

// exists returns whether the given file or directory exists
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func DownloadLDBCFiles(dataDir string, downloadResources bool) (string, error) {
	if !downloadResources {
		fmt.Print("Skipping downloading of resources\n")
		return dataDir, nil
	}
	if dataDir != "" {
		ok, err := exists(dataDir)
		if err != nil {
			return dataDir, errors.Wrapf(err, "while downloading data in %s", dataDir)
		}
		if ok {
			fmt.Print("Skipping downloading as files already present\n")
			return dataDir, nil
		}
	} else {
		dataDir = os.TempDir() + "/ldbcData"
	}

	x.Check(MakeDirEmpty([]string{dataDir}))

	for _, name := range rdfFileNames {
		filepath := BaseUrl + name + Suffix
		ldbcDataFiles[name] = filepath
	}

	start := time.Now()
	var eg errgroup.Group

	for fname, link := range ldbcDataFiles {
		fileName := fname
		fileLink := link
		eg.Go(func() error {
			start := time.Now()
			cmd := exec.Command("wget", "-O", fileName, fileLink)
			cmd.Dir = dataDir
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Printf("Error %v", err)
				fmt.Printf("Output %v", out)
				return errors.Wrapf(err, "while downloading %s", fileName)
			}
			fmt.Printf("Downloaded %s to %s in %s \n", fileName, dataDir, time.Since(start))
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return dataDir, err
	}

	fmt.Printf("Downloaded %d files in %s \n", len(ldbcDataFiles), time.Since(start))
	return dataDir, nil
}

// CompareJSON compares two JSON objects (passed as strings).
func CompareJSONBench(t *testing.B, want, got string) {
	wantMap := UnmarshalJSONBench(t, want)
	gotMap := UnmarshalJSONBench(t, got)
	CompareJSONMapsBench(t, wantMap, gotMap)
}

// CompareJSONMaps compares two JSON objects (passed as maps).
func CompareJSONMapsBench(t *testing.B, wantMap, gotMap map[string]interface{}) bool {
	return DiffJSONMapsBench(t, wantMap, gotMap, "", false)
}

// EqualJSON compares two JSON objects for equality.
func EqualJSONBench(t *testing.B, want, got string, savepath string, quiet bool) bool {
	wantMap := UnmarshalJSONBench(t, want)
	gotMap := UnmarshalJSONBench(t, got)

	return DiffJSONMapsBench(t, wantMap, gotMap, savepath, quiet)
}

// UnmarshalJSON unmarshals the given string into a map.
func UnmarshalJSONBench(t *testing.B, jsonStr string) map[string]interface{} {
	jsonMap := map[string]interface{}{}
	err := json.Unmarshal([]byte(jsonStr), &jsonMap)
	if err != nil {
		t.Fatalf("Could not unmarshal want JSON: %v", err)
	}

	return jsonMap
}

// DiffJSONMaps compares two JSON maps, optionally printing their differences,
// and returning true if they are equal.
func DiffJSONMapsBench(t *testing.B, wantMap, gotMap map[string]interface{},
	savepath string, quiet bool) bool {
	sortJSON(wantMap)
	sortJSON(gotMap)

	if !reflect.DeepEqual(wantMap, gotMap) {
		wantBuf, err := json.MarshalIndent(wantMap, "", "  ")
		if err != nil {
			t.Error("Could not marshal JSON:", err)
		}
		gotBuf, err := json.MarshalIndent(gotMap, "", "  ")
		if err != nil {
			t.Error("Could not marshal JSON:", err)
		}
		if !quiet {
			t.Errorf("Expected JSON and actual JSON differ:\n%s",
				sdiffJSON(wantBuf, gotBuf, savepath, quiet))
		}
		return false
	}

	return true
}
