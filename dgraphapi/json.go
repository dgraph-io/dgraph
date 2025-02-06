/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package dgraphapi

import (
	"encoding/json"
	"fmt"
	"hash/crc64"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"time"

	"github.com/pkg/errors"
)

const (
	waitDurBeforeRetry = time.Second
	requestTimeout     = 120 * time.Second
)

func PollTillPassOrTimeout(gcli *GrpcClient, query, want string, timeout time.Duration) error {
	ticker := time.NewTimer(requestTimeout)
	defer ticker.Stop()
	for {
		resp, err := gcli.Query(query)
		if err != nil {
			return errors.Wrap(err, "error while query")
		}

		err = CompareJSON(want, string(resp.Json))
		if err == nil {
			return nil
		}
		select {
		case <-ticker.C:
			return err
		default:
			time.Sleep(waitDurBeforeRetry)
		}
	}
}

// CompareJSON compares two JSON objects (passed as strings).
func CompareJSON(want, got string) error {
	wantMap, err := unmarshalJSON(want)
	if err != nil {
		return err
	}
	gotMap, err := unmarshalJSON(got)
	if err != nil {
		return err
	}
	return compareJSONMaps(wantMap, gotMap)
}

// compareJSONMaps compares two JSON objects (passed as maps).
func compareJSONMaps(wantMap, gotMap map[string]interface{}) error {
	return diffJSONMaps(wantMap, gotMap, "")
}

// unmarshalJSON unmarshals the given string into a map.
func unmarshalJSON(jsonStr string) (map[string]interface{}, error) {
	jsonMap := map[string]interface{}{}
	err := json.Unmarshal([]byte(jsonStr), &jsonMap)
	if err != nil {
		return nil, errors.Errorf("could not unmarshal want JSON: %v", err)
	}
	return jsonMap, nil
}

// diffJSONMaps compares two JSON maps, optionally printing their differences,
// and returning true if they are equal.
func diffJSONMaps(wantMap, gotMap map[string]interface{}, savepath string) error {
	sortJSON(wantMap)
	sortJSON(gotMap)
	if !reflect.DeepEqual(wantMap, gotMap) {
		wantBuf, err := json.MarshalIndent(wantMap, "", "  ")
		if err != nil {
			return errors.Wrap(err, "could not marshal JSON")
		}
		gotBuf, err := json.MarshalIndent(gotMap, "", "  ")
		if err != nil {
			return errors.Wrap(err, "could not marshal JSON")
		}
		return errors.Errorf("expected JSON and actual JSON differ:\n%s",
			sdiffJSON(wantBuf, gotBuf, savepath))
	}
	return nil
}

func sdiffJSON(wantBuf, gotBuf []byte, savepath string) string {
	var wantFile, gotFile *os.File

	if savepath != "" {
		_ = os.MkdirAll(filepath.Dir(savepath), 0700)
		wantFile, _ = os.Create(savepath + ".expected.json")
		gotFile, _ = os.Create(savepath + ".received.json")
	} else {
		wantFile, _ = os.CreateTemp("", "testutil.expected.json.*")
		defer os.RemoveAll(wantFile.Name())
		gotFile, _ = os.CreateTemp("", "testutil.expected.json.*")
		defer os.RemoveAll(gotFile.Name())
	}

	_ = os.WriteFile(wantFile.Name(), wantBuf, 0600)
	_ = os.WriteFile(gotFile.Name(), gotBuf, 0600)

	// don't do diff when one side is missing
	if len(gotBuf) == 0 {
		return "Got empty response"
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

func sortJSONArray(a []interface{}) uint64 {
	type arrayElement struct {
		elem   interface{}
		sortBy uint64
	}

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
