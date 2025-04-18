/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package hnsw

import (
	"context"
	"fmt"
	"sync"
	"testing"

	c "github.com/hypermodeinc/dgraph/v25/tok/constraints"
	"github.com/hypermodeinc/dgraph/v25/tok/index"
	opt "github.com/hypermodeinc/dgraph/v25/tok/options"
	"golang.org/x/exp/slices"
)

type createpersistentHNSWTest[T c.Float] struct {
	maxLevels         int
	efSearch          int
	efConstruction    int
	pred              string
	indexType         string
	expectedIndexType string
	floatBits         int
}

var createpersistentHNSWTests = []createpersistentHNSWTest[float64]{
	{
		maxLevels:         1,
		efSearch:          1,
		efConstruction:    1,
		pred:              "a",
		indexType:         "b",
		expectedIndexType: Euclidean,
		floatBits:         64,
	},
	{
		maxLevels:         1,
		efSearch:          1,
		efConstruction:    1,
		pred:              "a",
		indexType:         Euclidean,
		expectedIndexType: Euclidean,
		floatBits:         64,
	},
	{
		maxLevels:         1,
		efSearch:          1,
		efConstruction:    1,
		pred:              "a",
		indexType:         Cosine,
		expectedIndexType: Cosine,
		floatBits:         64,
	},
	{
		maxLevels:         1,
		efSearch:          1,
		efConstruction:    1,
		pred:              "a",
		indexType:         DotProd,
		expectedIndexType: DotProd,
		floatBits:         64,
	},
}

func optionsFromCreateTestCase[T c.Float](tc createpersistentHNSWTest[T]) opt.Options {
	retVal := opt.NewOptions()
	retVal.SetOpt(MaxLevelsOpt, tc.maxLevels)
	retVal.SetOpt(EfSearchOpt, tc.efSearch)
	retVal.SetOpt(EfConstructionOpt, tc.efConstruction)
	retVal.SetOpt(MetricOpt, GetSimType[T](tc.indexType, tc.floatBits))
	return retVal
}

func TestRaceCreateOrReplace(t *testing.T) {
	f := CreateFactory[float64](64)
	test := createpersistentHNSWTests[0]
	opts := optionsFromCreateTestCase(test)

	var wg sync.WaitGroup
	run := func() {
		for i := range 10 {
			vIndex, err := f.CreateOrReplace(test.pred, opts, 32)
			if err != nil {
				t.Errorf("Error creating index: %s for test case %d (%+v)",
					err, i, test)
			}
			if vIndex == nil {
				t.Errorf("TestCreatepersistentHNSW test case %d (%+v) generated nil index",
					i, test)
			}
		}
		wg.Done()
	}

	for range 5 {
		wg.Add(1)
		go run()
	}

	wg.Wait()
}

func TestCreatepersistentHNSW(t *testing.T) {
	f := CreateFactory[float64](32)
	for i, test := range createpersistentHNSWTests {
		opts := optionsFromCreateTestCase(test)
		vIndex, err := f.CreateOrReplace(test.pred, opts, 32)
		if err != nil {
			t.Errorf("Error creating index: %s for test case %d (%+v)",
				err, i, test)
			return
		}
		if vIndex == nil {
			t.Errorf("TestCreatepersistentHNSW test case %d (%+v) generated nil index",
				i, test)
			return
		}
		flatPh := vIndex.(*persistentHNSW[float64])
		if flatPh.simType.indexType != test.expectedIndexType {
			t.Errorf("output %q not equal to expected %q", flatPh.simType.indexType, test.expectedIndexType)
			return
		}
	}
}

type flatInMemListAddMutationTest struct {
	key         string
	startTs     uint64
	finishTs    uint64
	t           *index.KeyValue
	expectedErr error
}

var flatInMemListAddMutationTests = []flatInMemListAddMutationTest{
	{key: "a", startTs: 0, finishTs: 5, t: &index.KeyValue{Value: []byte("abc")}, expectedErr: nil},
	{key: "b", startTs: 1, finishTs: 2, t: &index.KeyValue{Value: []byte("123")}, expectedErr: nil},
	{key: "c", startTs: 0, finishTs: 99, t: &index.KeyValue{Value: []byte("xyz")}, expectedErr: nil},
}

// TODO: It seriously seems wrong to have a transactional concept so tightly coupled with
//
//	Dgraph product here! We should expect that we are using this module for completely
//	independent use, possibly having nothing to do with Dgraph.
func flatInMemListWriteMutation(test flatInMemListAddMutationTest, t *testing.T) {
	l := newInMemList(test.key, test.startTs, test.finishTs)
	err := l.AddMutation(context.TODO(), nil, test.t)
	if err != nil {
		if err.Error() != test.expectedErr.Error() {
			t.Errorf("Output %q not equal to expected %q", err.Error(), test.expectedErr.Error())
		}
	} else {
		if err != test.expectedErr {
			t.Errorf("Output %q not equal to expected %q", err, test.expectedErr)
		}
	}
	// should not modify db [test.startTs, test.finishTs)
	if string(tsDbs[test.finishTs-1].inMemTestDb[test.key]) != string(tsDbs[test.startTs].inMemTestDb[test.key]) {
		t.Errorf(
			"Database at time %q not equal to expected database at time %q. Expected: %q, Got: %q",
			test.finishTs-1, test.startTs,
			tsDbs[test.startTs].inMemTestDb[test.key],
			tsDbs[test.finishTs-1].inMemTestDb[test.key])
	}
	if string(tsDbs[test.finishTs].inMemTestDb[test.key][:]) != string(test.t.Value[:]) {
		t.Errorf("The database at time %q for key %q gave value  of %q instead of %q", test.finishTs,
			test.key, string(tsDbs[test.finishTs].inMemTestDb[test.key][:]), string(test.t.Value[:]))
	}
	if string(tsDbs[test.finishTs].inMemTestDb[test.key][:]) !=
		string(tsDbs[99].inMemTestDb[test.key][:]) {
		t.Errorf("The database at time %q for key %q gave value  of %q instead of %q", test.finishTs,
			test.key, string(tsDbs[99].inMemTestDb[test.key][:]),
			string(tsDbs[test.finishTs].inMemTestDb[test.key][:]))
	}
}

func TestFlatInMemListAddMutation(t *testing.T) {
	emptyTsDbs()
	for _, test := range flatInMemListAddMutationTests {
		flatInMemListWriteMutation(test, t)
	}
}

var flatInMemListAddMutationOverwriteTests = []flatInMemListAddMutationTest{
	{key: "a", startTs: 0, finishTs: 5, t: &index.KeyValue{Value: []byte("abc")}, expectedErr: nil},
	{key: "a", startTs: 0, finishTs: 5, t: &index.KeyValue{Value: []byte("123")}, expectedErr: nil},
	{key: "a", startTs: 0, finishTs: 5, t: &index.KeyValue{Value: []byte("xyz")}, expectedErr: nil},
}

func TestFlatInMemListAddOverwriteMutation(t *testing.T) {
	emptyTsDbs()
	for _, test := range flatInMemListAddMutationOverwriteTests {
		flatInMemListWriteMutation(test, t)
	}
}

type flatInMemListAddMutationTestBranchDependent struct {
	key           string
	startTs       uint64
	finishTs      uint64
	t             *index.KeyValue
	expectedErr   error
	currIteration int
}

var flatInMemListAddMultipleWritesMutationTests = []flatInMemListAddMutationTestBranchDependent{
	{key: "a", startTs: 0, finishTs: 2, t: &index.KeyValue{Value: []byte("abc")}, expectedErr: nil, currIteration: 0},
	{key: "a", startTs: 1, finishTs: 3, t: &index.KeyValue{Value: []byte("123")}, expectedErr: nil, currIteration: 1},
	{key: "a", startTs: 2, finishTs: 4, t: &index.KeyValue{Value: []byte("xyz")}, expectedErr: nil, currIteration: 2},
}

func TestFlatInMemListAddMultipleWritesMutation(t *testing.T) {
	emptyTsDbs()
	for _, test := range flatInMemListAddMultipleWritesMutationTests {
		l := newInMemList(test.key, test.startTs, test.finishTs)
		err := l.AddMutation(context.TODO(), nil, test.t)
		if err != nil {
			if err.Error() != test.expectedErr.Error() {
				t.Errorf("Output %q not equal to expected %q", err.Error(), test.expectedErr.Error())
			}
		} else {
			if err != test.expectedErr {
				t.Errorf("Output %q not equal to expected %q", err, test.expectedErr)
			}
		}
		if test.currIteration == 0 {
			conv := flatInMemListAddMutationTest{test.key, test.startTs, test.finishTs, test.t, test.expectedErr}
			flatInMemListWriteMutation(conv, t)
		} else {
			if string(tsDbs[test.finishTs-1].inMemTestDb[test.key][:]) !=
				string(flatInMemListAddMultipleWritesMutationTests[test.currIteration-1].t.Value[:]) {
				t.Errorf("The database at time %q for key %q gave value  of %q instead of %q", test.finishTs,
					test.key, string(tsDbs[test.finishTs].inMemTestDb[test.key][:]), string(test.t.Value[:]))
			}
			if string(tsDbs[test.finishTs].inMemTestDb[test.key][:]) != string(test.t.Value[:]) {
				t.Errorf("The database at time %q for key %q gave value  of %q instead of %q", test.finishTs,
					test.key, string(tsDbs[test.finishTs].inMemTestDb[test.key][:]), string(test.t.Value[:]))
			}
		}
	}
}

type insertToPersistentFlatStorageTest struct {
	tc                *TxnCache
	inUuid            uint64
	inVec             []float64
	expectedErr       error
	expectedEdgesList []string
	minExpectedEdge   string
}

var flatPhs = []*persistentHNSW[float64]{
	{
		maxLevels:      5,
		efConstruction: 16,
		efSearch:       12,
		pred:           "0-a",
		vecEntryKey:    ConcatStrings("0-a", VecEntry),
		vecKey:         ConcatStrings("0-a", VecKeyword),
		vecDead:        ConcatStrings("0-a", VecDead),
		floatBits:      64,
		simType:        GetSimType[float64](Euclidean, 64),
		nodeAllEdges:   make(map[uint64][][]uint64),
	},
	{
		maxLevels:      5,
		efConstruction: 16,
		efSearch:       12,
		pred:           "0-a",
		vecEntryKey:    ConcatStrings("0-a", VecEntry),
		vecKey:         ConcatStrings("0-a", VecKeyword),
		vecDead:        ConcatStrings("0-a", VecDead),
		floatBits:      64,
		simType:        GetSimType[float64](Cosine, 64),
		nodeAllEdges:   make(map[uint64][][]uint64),
	},
	{
		maxLevels:      5,
		efConstruction: 16,
		efSearch:       12,
		pred:           "0-a",
		vecEntryKey:    ConcatStrings("0-a", VecEntry),
		vecKey:         ConcatStrings("0-a", VecKeyword),
		vecDead:        ConcatStrings("0-a", VecDead),
		floatBits:      64,
		simType:        GetSimType[float64](DotProd, 64),
		nodeAllEdges:   make(map[uint64][][]uint64),
	},
}

var flatEntryInsertToPersistentFlatStorageTests = []insertToPersistentFlatStorageTest{
	{
		tc:                NewTxnCache(&inMemTxn{startTs: 12, commitTs: 40}, 12),
		inUuid:            uint64(123),
		inVec:             []float64{0.824, 0.319, 0.111},
		expectedErr:       nil,
		expectedEdgesList: []string{"0-a__vector__123", "0-a__vector_entry_1"},
		minExpectedEdge:   "",
	},
	{
		tc:                NewTxnCache(&inMemTxn{startTs: 11, commitTs: 37}, 11),
		inUuid:            uint64(1),
		inVec:             []float64{0.3, 0.5, 0.7},
		expectedErr:       nil,
		expectedEdgesList: []string{"0-a__vector__1", "0-a__vector_entry_1"},
		minExpectedEdge:   "",
	},
	{
		tc:                NewTxnCache(&inMemTxn{startTs: 0, commitTs: 1}, 0),
		inUuid:            uint64(5),
		inVec:             []float64{0.1, 0.1, 0.1},
		expectedErr:       nil,
		expectedEdgesList: []string{"0-a__vector__5", "0-a__vector_entry_1"},
		minExpectedEdge:   "",
	},
}

func TestFlatEntryInsertToPersistentFlatStorage(t *testing.T) {
	emptyTsDbs()
	flatPh := flatPhs[0]
	for _, test := range flatEntryInsertToPersistentFlatStorageTests {
		emptyTsDbs()
		key := DataKey(flatPh.pred, test.inUuid)
		for i := range tsDbs {
			tsDbs[i].inMemTestDb[string(key[:])] = floatArrayAsBytes(test.inVec)
		}
		edges, err := flatPh.Insert(context.TODO(), test.tc, test.inUuid, test.inVec)
		if err != nil {
			if err.Error() != test.expectedErr.Error() {
				t.Errorf("Output %q not equal to expected %q", err.Error(), test.expectedErr.Error())
			}
		} else {
			if err != test.expectedErr {
				t.Errorf("Output %q not equal to expected %q", err, test.expectedErr)
			}
		}
		var float1, float2 = []float64{}, []float64{}
		skey := string(key[:])
		index.BytesAsFloatArray(tsDbs[0].inMemTestDb[skey], &float1, 64)
		index.BytesAsFloatArray(tsDbs[99].inMemTestDb[skey], &float2, 64)
		if !equalFloat64Slice(float1, float2) {
			t.Errorf("Vector value for predicate %q at beginning and end of database were "+
				"not equivalent. Start Value: %v\n, End Value: %v\n %v\n %v", flatPh.pred, tsDbs[0].inMemTestDb[skey],
				tsDbs[99].inMemTestDb[skey], float1, float2)
		}
		edgesNameList := []string{}
		for _, edge := range edges {
			edgeName := edge.Attr + "_" + fmt.Sprint(edge.Entity)
			edgesNameList = append(edgesNameList, edgeName)
		}
		if !equalStringSlice(edgesNameList, test.expectedEdgesList) {
			t.Errorf("Edges created during insert is incorrect. Expected: %v, Got: %v", test.expectedEdgesList, edgesNameList)
		}
		entryKey := DataKey(ConcatStrings(flatPh.pred, VecEntry), 1)
		entryVal := BytesToUint64(tsDbs[99].inMemTestDb[string(entryKey[:])])
		if entryVal != test.inUuid {
			t.Errorf("entry value stored is incorrect. Expected: %q, Got: %q", test.inUuid, entryVal)
		}
	}
}

var flatEntryInsert = insertToPersistentFlatStorageTest{
	tc:          NewTxnCache(&inMemTxn{startTs: 0, commitTs: 1}, 0),
	inUuid:      uint64(5),
	inVec:       []float64{0.1, 0.1, 0.1},
	expectedErr: nil,
	expectedEdgesList: []string{
		"0-a__vector__5",
		"0-a__vector__5",
		"0-a__vector__5",
		"0-a__vector__5",
		"0-a__vector__5",
		"0-a__vector_entry_1",
	},
	minExpectedEdge: "",
}

var nonflatEntryInsertToPersistentFlatStorageTests = []insertToPersistentFlatStorageTest{
	{
		tc:                NewTxnCache(&inMemTxn{startTs: 12, commitTs: 40}, 12),
		inUuid:            uint64(123),
		inVec:             []float64{0.824, 0.319, 0.111},
		expectedErr:       nil,
		expectedEdgesList: []string{},
		minExpectedEdge:   "0-a__vector__123",
	},
	{
		tc:                NewTxnCache(&inMemTxn{startTs: 11, commitTs: 37}, 11),
		inUuid:            uint64(1),
		inVec:             []float64{0.3, 0.5, 0.7},
		expectedErr:       nil,
		expectedEdgesList: []string{},
		minExpectedEdge:   "0-a__vector__1",
	},
}

func TestNonflatEntryInsertToPersistentFlatStorage(t *testing.T) {
	emptyTsDbs()
	flatPh := flatPhs[0]
	key := DataKey(flatPh.pred, flatEntryInsert.inUuid)
	for i := range tsDbs {
		tsDbs[i].inMemTestDb[string(key[:])] = floatArrayAsBytes(flatEntryInsert.inVec)
	}
	_, err := flatPh.Insert(context.TODO(),
		flatEntryInsert.tc,
		flatEntryInsert.inUuid,
		flatEntryInsert.inVec)
	if err != nil {
		t.Errorf("Encountered error on initial insert: %s", err)
		return
	}
	// testKey := DataKey(BuildDataKeyPred(flatPh.pred, VecKeyword, fmt.Sprint(0)), flatEntryInsert.inUuid)
	// fmt.Print(tsDbs[1].inMemTestDb[string(testKey[:])])
	for _, test := range nonflatEntryInsertToPersistentFlatStorageTests {
		entryKey := DataKey(ConcatStrings(flatPh.pred, VecEntry), 1)
		entryVal := BytesToUint64(tsDbs[99].inMemTestDb[string(entryKey[:])])
		if entryVal != 5 {
			t.Errorf("entry value stored is incorrect. Expected: %q, Got: %q", 5, entryVal)
		}
		for i := range tsDbs {
			key := DataKey(flatPh.pred, test.inUuid)
			tsDbs[i].inMemTestDb[string(key[:])] = floatArrayAsBytes(test.inVec)
		}
		edges, err := flatPh.Insert(context.TODO(), test.tc, test.inUuid, test.inVec)
		if err != nil && test.expectedErr != nil {
			if err.Error() != test.expectedErr.Error() {
				t.Errorf("Output %q not equal to expected %q", err.Error(), test.expectedErr.Error())
			}
		} else {
			if err != test.expectedErr {
				t.Errorf("Output %q not equal to expected %q", err, test.expectedErr)
			}
		}
		var float1, float2 = []float64{}, []float64{}
		index.BytesAsFloatArray(tsDbs[0].inMemTestDb[string(key[:])], &float1, 64)
		index.BytesAsFloatArray(tsDbs[99].inMemTestDb[string(key[:])], &float2, 64)
		if !equalFloat64Slice(float1, float2) {
			t.Errorf("Vector value for predicate %q at beginning and end of database were "+
				"not equivalent. Start Value: %v, End Value: %v", flatPh.pred, tsDbs[0].inMemTestDb[flatPh.pred],
				tsDbs[99].inMemTestDb[flatPh.pred])
		}
		edgesNameList := []string{}
		for _, edge := range edges {
			edgeName := edge.Attr + "_" + fmt.Sprint(edge.Entity)
			edgesNameList = append(edgesNameList, edgeName)
		}
		if !slices.Contains(edgesNameList, test.minExpectedEdge) {
			t.Errorf("Expected at least %q in list of edges %v", test.minExpectedEdge, edgesNameList)
		}
	}
}

type searchPersistentFlatStorageTest struct {
	qc          *QueryCache
	query       []float64
	maxResults  int
	expectedErr error
	expectedNns []uint64
}

var searchPersistentFlatStorageTests = []searchPersistentFlatStorageTest{
	{
		qc:          NewQueryCache(&inMemLocalCache{readTs: 45}, 45),
		query:       []float64{0.3, 0.5, 0.7},
		maxResults:  1,
		expectedErr: nil,
		expectedNns: []uint64{1},
	},
	{
		qc:          NewQueryCache(&inMemLocalCache{readTs: 93}, 93),
		query:       []float64{0.824, 0.319, 0.111},
		maxResults:  1,
		expectedErr: nil,
		expectedNns: []uint64{123},
	},
}

var flatPopulateBasicInsertsForSearch = []insertToPersistentFlatStorageTest{
	{
		tc:                NewTxnCache(&inMemTxn{startTs: 0, commitTs: 1}, 0),
		inUuid:            uint64(5),
		inVec:             []float64{0.1, 0.1, 0.1},
		expectedErr:       nil,
		expectedEdgesList: nil,
		minExpectedEdge:   "",
	},
	{
		tc:                NewTxnCache(&inMemTxn{startTs: 11, commitTs: 15}, 11),
		inUuid:            uint64(123),
		inVec:             []float64{0.824, 0.319, 0.111},
		expectedErr:       nil,
		expectedEdgesList: nil,
		minExpectedEdge:   "",
	},
	{
		tc:                NewTxnCache(&inMemTxn{startTs: 20, commitTs: 37}, 20),
		inUuid:            uint64(1),
		inVec:             []float64{0.3, 0.5, 0.7},
		expectedErr:       nil,
		expectedEdgesList: nil,
		minExpectedEdge:   "",
	},
}

func flatPopulateInserts(insertArr []insertToPersistentFlatStorageTest, flatPh *persistentHNSW[float64]) error {
	emptyTsDbs()
	for _, in := range insertArr {
		for i := range tsDbs {
			key := DataKey(flatPh.pred, in.inUuid)
			tsDbs[i].inMemTestDb[string(key[:])] = floatArrayAsBytes(in.inVec)
		}
		_, err := flatPh.Insert(context.TODO(), in.tc, in.inUuid, in.inVec)
		if err != nil {
			return err
		}
	}
	return nil
}

func RunFlatSearchTests(t *testing.T, test searchPersistentFlatStorageTest, flatPh *persistentHNSW[float64]) {
	nns, err := flatPh.Search(context.TODO(), test.qc, test.query, test.maxResults, index.AcceptAll[float64])
	if err != nil && test.expectedErr != nil {
		if err.Error() != test.expectedErr.Error() {
			t.Errorf("Output %q not equal to expected %q", err.Error(), test.expectedErr.Error())
		}
	} else {
		if err != test.expectedErr {
			t.Errorf("Output %q not equal to expected %q", err, test.expectedErr)
		}
	}
	if !equalUint64Slice(nns, test.expectedNns) {
		t.Errorf("Nearest neighbors expected value: %v, Got: %v", test.expectedNns, nns)
	}
}

func TestBasicSearchPersistentFlatStorage(t *testing.T) {
	for _, flatPh := range flatPhs {
		emptyTsDbs()
		err := flatPopulateInserts(flatPopulateBasicInsertsForSearch, flatPh)
		if err != nil {
			t.Errorf("Error populating inserts: %s", err)
			return
		}
		for _, test := range searchPersistentFlatStorageTests {
			RunFlatSearchTests(t, test, flatPh)
		}
	}
}
