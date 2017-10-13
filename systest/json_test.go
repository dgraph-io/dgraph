package main

import (
	"encoding/json"
	"fmt"
	"hash/crc64"
	"reflect"
	"sort"
	"testing"
)

func CompareJSON(t *testing.T, want, got string) {
	wantMap := map[string]interface{}{}
	err := json.Unmarshal([]byte(want), &wantMap)
	if err != nil {
		t.Fatalf("Could not unmarshal want JSON: %v", err)
	}
	gotMap := map[string]interface{}{}
	err = json.Unmarshal([]byte(got), &gotMap)
	if err != nil {
		t.Fatalf("Could not unmarshal got JSON: %v", err)
	}

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
		t.Errorf("Want JSON and Got JSON not equal\nWant:\n%v\nGot:\n%v",
			string(wantBuf), string(gotBuf))
	}
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
	h := uint64(0)
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
	h := uint64(0)
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
