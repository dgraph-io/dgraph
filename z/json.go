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

package z

import (
	"encoding/json"
	"fmt"
	"hash/crc64"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"reflect"
	"sort"
	"testing"
)

// CompareJSON compares two JSON objects (passed as strings).
func CompareJSON(t *testing.T, want, got string) {
	wantMap := UnmarshalJSON(t, want)
	gotMap := UnmarshalJSON(t, got)
	CompareJSONMaps(t, wantMap, gotMap)
}

// CompareJSON compares two JSON objects (passed as maps).
func CompareJSONMaps(t *testing.T, wantMap, gotMap map[string]interface{}) bool {
	return DiffJSONMaps(t, wantMap, gotMap, "", false)
}

//EqualJSON compares two JSON objects for equality.
func EqualJSON(t *testing.T, want, got string, savepath string, quiet bool) bool {
	wantMap := UnmarshalJSON(t, want)
	gotMap := UnmarshalJSON(t, got)

	return DiffJSONMaps(t, wantMap, gotMap, savepath, quiet)
}

// UnmarshalJSON unmarshals the given string into a map.
func UnmarshalJSON(t *testing.T, jsonStr string) map[string]interface{} {
	jsonMap := map[string]interface{}{}
	err := json.Unmarshal([]byte(jsonStr), &jsonMap)
	if err != nil {
		t.Fatalf("Could not unmarshal want JSON: %v", err)
	}

	return jsonMap
}

// DiffJSONMaps compares two JSON maps, optionally printing their differences,
// and returning true if they are equal.
func DiffJSONMaps(t *testing.T, wantMap, gotMap map[string]interface{},
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
		t.Errorf("Expected JSON and actual JSON differ:\n%s",
			sdiffJSON(wantBuf, gotBuf, savepath, quiet))
		return false
	}

	return true
}

// SnipJSON snips the middle of a very long JSON string to make it less than 100 lines
func SnipJSON(buf []byte) string {
	var n int
	for i, ch := range buf {
		if ch == '\n' {
			if n < 100 {
				if n == 99 && i != len(buf) {
					i++
					return string(buf[:i]) + fmt.Sprintf("[%d bytes snipped]", len(buf)-i)
				}
				n++
			}
		}
	}
	return string(buf)
}

func sdiffJSON(wantBuf, gotBuf []byte, savepath string, quiet bool) string {
	var wantFile, gotFile *os.File

	if savepath != "" {
		_ = os.MkdirAll(path.Dir(savepath), 0700)
		wantFile, _ = os.Create(savepath + ".expected.json")
		gotFile, _ = os.Create(savepath + ".received.json")
	} else {
		wantFile, _ = ioutil.TempFile("", "z.expected.json.*")
		defer os.RemoveAll(wantFile.Name())
		gotFile, _ = ioutil.TempFile("", "z.expected.json.*")
		defer os.RemoveAll(gotFile.Name())
	}

	_ = ioutil.WriteFile(wantFile.Name(), wantBuf, 0600)
	_ = ioutil.WriteFile(gotFile.Name(), gotBuf, 0600)

	// don't do diff when one side is missing
	if len(gotBuf) == 0 {
		return "Got empty response"
	} else if quiet {
		return "Not showing diff in quiet mode"
	}

	out, _ := exec.Command("sdiff", wantFile.Name(), gotFile.Name()).CombinedOutput()

	return string(out)
}

// sortJSON looks for any arrays in the unmarshalled JSON and sorts them in an
// arbitrary but deterministic order based on their content.
func sortJSON(i interface{}) uint64 {
	if i == nil {
		return 0
	}
	switch i := i.(type) {
	case map[string]interface{}:
		return sortJSONMap(i)
	case []interface{}:
		return sortJSONArray(i)
	default:
		h := crc64.New(crc64.MakeTable(crc64.ISO))
		fmt.Fprint(h, i)
		return h.Sum64()
	}
}

func sortJSONMap(m map[string]interface{}) uint64 {
	var h uint64
	for _, k := range m {
		// Because xor is commutative, it doesn't matter that map iteration
		// is in random order.
		h ^= sortJSON(k)
	}
	return h
}

type arrayElement struct {
	elem   interface{}
	sortBy uint64
}

func sortJSONArray(a []interface{}) uint64 {
	var h uint64
	elements := make([]arrayElement, len(a))
	for i, elem := range a {
		elements[i] = arrayElement{elem, sortJSON(elem)}
		h ^= elements[i].sortBy
	}
	sort.Slice(elements, func(i, j int) bool {
		return elements[i].sortBy < elements[j].sortBy
	})
	for i := range a {
		a[i] = elements[i].elem
	}
	return h
}
