//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
	"github.com/hypermodeinc/dgraph/v25/x"
	"github.com/stretchr/testify/require"
)

var (
	vectorSchemaWithIndex = `%v: float32vector @index(hnsw(exponent: "%v", metric: "%v")) .`
)

const (
	vectorSchemaWithoutIndex = `%v: float32vector .`
)

var client *dgraphapi.GrpcClient
var dc dgraphapi.Cluster

func setSchema(schema string) {
	var err error
	for retry := 0; retry < 60; retry++ {
		err = client.Alter(context.Background(), &api.Operation{Schema: schema})
		if err == nil {
			return
		}
		time.Sleep(time.Second)
	}
	panic(fmt.Sprintf("Could not alter schema. Got error %v", err.Error()))
}

func dropPredicate(pred string) {
	err := client.Alter(context.Background(), &api.Operation{
		DropAttr: pred,
	})
	if err != nil {
		panic(fmt.Sprintf("Could not drop predicate. Got error %v", err.Error()))
	}
}

func processQuery(ctx context.Context, t *testing.T, query string) (string, error) {
	txn := client.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			t.Logf("error discarding txn: %v", err)
		}
	}()

	res, err := txn.Query(ctx, query)
	if err != nil {
		return "", err
	}

	response := map[string]interface{}{}
	response["data"] = json.RawMessage(string(res.Json))

	jsonResponse, err := json.Marshal(response)
	require.NoError(t, err)
	return string(jsonResponse), err
}

func processQueryRDF(ctx context.Context, t *testing.T, query string) (string, error) {
	txn := client.NewTxn()
	defer func() { _ = txn.Discard(ctx) }()

	res, err := txn.Do(ctx, &api.Request{
		Query:      query,
		RespFormat: api.Request_RDF,
	})
	if err != nil {
		return "", err
	}
	return string(res.Rdf), err
}

func processQueryNoErr(t *testing.T, query string) string {
	res, err := processQuery(context.Background(), t, query)
	require.NoError(t, err)
	return res
}

// processQueryForMetrics works like processQuery but returns metrics instead of response.
func processQueryForMetrics(t *testing.T, query string) *api.Metrics {
	txn := client.NewTxn()
	defer func() { _ = txn.Discard(context.Background()) }()

	res, err := txn.Query(context.Background(), query)
	require.NoError(t, err)
	return res.Metrics
}

func processQueryWithVars(t *testing.T, query string,
	vars map[string]string) (string, error) {
	txn := client.NewTxn()
	defer func() { _ = txn.Discard(context.Background()) }()

	res, err := txn.QueryWithVars(context.Background(), query, vars)
	if err != nil {
		return "", err
	}

	response := map[string]interface{}{}
	response["data"] = json.RawMessage(string(res.Json))

	jsonResponse, err := json.Marshal(response)
	require.NoError(t, err)
	return string(jsonResponse), err
}

func addTriplesToCluster(triples string) error {
	txn := client.NewTxn()
	ctx := context.Background()
	defer func() { _ = txn.Discard(ctx) }()

	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(triples),
		CommitNow: true,
	})
	return err
}

func deleteTriplesInCluster(triples string) {
	txn := client.NewTxn()
	ctx := context.Background()
	defer func() { _ = txn.Discard(ctx) }()

	_, err := txn.Mutate(ctx, &api.Mutation{
		DelNquads: []byte(triples),
		CommitNow: true,
	})
	if err != nil {
		panic(fmt.Sprintf("Could not delete triples. Got error %v", err.Error()))
	}
}
func updateVector(t *testing.T, triple string, pred string) []float32 {
	uid := strings.Split(triple, " ")[0]
	randomVec := generateRandomVector(10)
	updatedTriple := fmt.Sprintf("%s <%s> \"%v\" .", uid, pred, randomVec)
	require.NoError(t, addTriplesToCluster(updatedTriple))

	updatedVec, err := queryVectorUsingUid(t, uid, pred)
	require.NoError(t, err)
	require.Equal(t, randomVec, updatedVec)
	return updatedVec
}

func queryVectorUsingUid(t *testing.T, uid, pred string) ([]float32, error) {
	vectorQuery := fmt.Sprintf(`
	 {
		 vector(func: uid(%v)) {
				%v
		  }
	 }`, uid, pred)

	resp, err := client.Query(vectorQuery)
	require.NoError(t, err)

	type VectorData struct {
		VTest []float32 `json:"vtest"`
	}

	type Data struct {
		Vector []VectorData `json:"vector"`
	}

	var data Data

	err = json.Unmarshal([]byte(resp.Json), &data)
	if err != nil {
		return []float32{}, err
	}

	return data.Vector[0].VTest, nil

}

func queryMultipleVectorsUsingSimilarTo(t *testing.T, vector []float32, pred string, topK int) ([][]float32, error) {
	vectorQuery := fmt.Sprintf(`
	 {
		 vector(func: similar_to(%v, %v, "%v")) {
				uid
				%v
		  }
	 }`, pred, topK, vector, pred)

	resp, err := client.Query(vectorQuery)
	require.NoError(t, err)

	type VectorData struct {
		UID   string    `json:"uid"`
		VTest []float32 `json:"vtest"`
	}

	type Data struct {
		Vector []VectorData `json:"vector"`
	}

	var data Data

	err = json.Unmarshal([]byte(resp.Json), &data)
	if err != nil {
		return [][]float32{}, err
	}

	var vectors [][]float32
	for _, vector := range data.Vector {
		vectors = append(vectors, vector.VTest)
	}
	return vectors, nil
}

func querySingleVectorError(t *testing.T, vector, pred string, validateError bool) ([]float32, error) {

	vectorQuery := fmt.Sprintf(`
	 {
		 vector(func: similar_to(%v, 1, "%v")) {
				uid
				%v
		  }
	 }`, pred, vector, pred)

	resp, err := client.Query(vectorQuery)
	if validateError {
		require.NoError(t, err)
	} else if err != nil {
		return []float32{}, err
	}

	type VectorData struct {
		UID   string    `json:"uid"`
		VTest []float32 `json:"vtest"`
	}

	type Data struct {
		Vector []VectorData `json:"vector"`
	}

	var data Data

	err = json.Unmarshal([]byte(resp.Json), &data)
	if err != nil {
		return []float32{}, err
	}

	if len(data.Vector) == 0 {
		return []float32{}, nil
	}

	return data.Vector[0].VTest, nil
}

func querySingleVector(t *testing.T, vector, pred string) ([]float32, error) {
	return querySingleVectorError(t, vector, pred, true)
}

func queryAllVectorsPred(t *testing.T, pred string) ([][]float32, error) {
	vectorQuery := fmt.Sprintf(`
	 {
		 vector(func: has(%v)) {
				uid
				%v
		  }
	 }`, pred, pred)

	resp, err := client.Query(vectorQuery)
	require.NoError(t, err)

	type VectorData struct {
		UID   string    `json:"uid"`
		VTest []float32 `json:"vtest"`
	}

	type Data struct {
		Vector []VectorData `json:"vector"`
	}

	var data Data

	err = json.Unmarshal([]byte(resp.Json), &data)
	if err != nil {
		return [][]float32{}, err
	}

	var vectors [][]float32
	for _, vector := range data.Vector {
		vectors = append(vectors, vector.VTest)
	}
	return vectors, nil
}

func generateRandomVector(size int) []float32 {
	vector := make([]float32, size)
	for i := 0; i < size; i++ {
		vector[i] = rand.Float32() * 10
	}
	return vector
}

func formatVector(label string, vector []float32, index int) string {
	vectorString := fmt.Sprintf(`"[%s]"`, strings.Trim(strings.Join(strings.Fields(fmt.Sprint(vector)), ", "), "[]"))
	return fmt.Sprintf("<0x%x> <%s> %s . \n", index+10, label, vectorString)
}

func generateRandomVectors(numVectors, vectorSize int, label string) (string, [][]float32) {
	var builder strings.Builder
	var vectors [][]float32
	// builder.WriteString("`")
	for i := 0; i < numVectors; i++ {
		randomVector := generateRandomVector(vectorSize)
		vectors = append(vectors, randomVector)
		formattedVector := formatVector(label, randomVector, i)
		builder.WriteString(formattedVector)
	}

	return builder.String(), vectors
}

func testVectorMutationSameLength(t *testing.T) {
	rdf, vectors := generateRandomVectors(10, 5, "vtest")
	require.NoError(t, addTriplesToCluster(rdf))

	allVectors, err := queryAllVectorsPred(t, "vtest")
	require.NoError(t, err)

	require.Equal(t, vectors, allVectors)

	triple := strings.Split(rdf, "\n")[1]
	vector, err := querySingleVector(t, strings.Split(triple, `"`)[1], "vtest")
	require.NoError(t, err)
	require.Contains(t, allVectors, vector)

	triple = strings.Split(rdf, "\n")[3]
	vector, err = querySingleVector(t, strings.Split(triple, `"`)[1], "vtest")
	require.NoError(t, err)
	require.Contains(t, allVectors, vector)

	triple = strings.Split(rdf, "\n")[5]
	vector, err = querySingleVector(t, strings.Split(triple, `"`)[1], "vtest")
	require.NoError(t, err)
	require.Contains(t, allVectors, vector)

	triple = strings.Split(rdf, "\n")[7]
	vector, err = querySingleVector(t, strings.Split(triple, `"`)[1], "vtest")
	require.NoError(t, err)
	require.Contains(t, allVectors, vector)

	triple = strings.Split(rdf, "\n")[9]
	vector, err = querySingleVector(t, strings.Split(triple, `"`)[1], "vtest")
	require.NoError(t, err)
	require.Contains(t, allVectors, vector)
}

func testVectorMutationDiffrentLength(t *testing.T, err string) {
	rdf := `<0x1> <vtest> "[1.5]" .
	 <0x2> <vtest> "[1.5, 2.0]" .
	 <0x3> <vtest> "[1.5, 2.0, 3.0]" .
	 <0x4> <vtest> "[1.5, 2.0, 3.0, 4.5]" .
	 <0x5> <vtest> "[1.5, 2.0, 3.0, 4.5, 5.0]" .
	 <0x6> <vtest> "[1.5, 2.0, 3.0, 4.5, 5.0, 6.5]" .
	 <0x7> <vtest> "[1.5, 2.0, 3.0, 4.5, 5.0, 6.5, 7.0]" .
	 <0x8> <vtest> "[1.5, 2.0, 3.0, 4.5, 5.0, 6.5, 7.0, 8.5]" .
	 <0x9> <vtest> "[1.5, 2.0, 3.0, 4.5, 5.0, 6.5, 7.0, 8.5, 9.0]" .
	 <0xA> <vtest> "[1.5, 2.0, 3.0, 4.5, 5.0, 6.5, 7.0, 8.5, 9.0, 10.5]" .`

	require.ErrorContains(t, addTriplesToCluster(rdf), err)
}

func TestInvalidVectorIndex(t *testing.T) {
	dropPredicate("vtest")
	schema := fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "euclidan")
	var err error
	for retry := 0; retry < 10; retry++ {
		err = client.Alter(context.Background(), &api.Operation{Schema: schema})
		if err == nil {
			require.Error(t, err)
		}
		if strings.Contains(err.Error(), "Can't create a vector index for euclidan") {
			return
		}
		time.Sleep(time.Second)
	}
	require.Error(t, nil)
}

func TestVectorIndexRebuildWhenChange(t *testing.T) {
	dropPredicate("vtest")
	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "euclidean"))

	numVectors := 9000
	vectorSize := 100

	randomVectors, _ := generateRandomVectors(numVectors, vectorSize, "vtest")
	require.NoError(t, addTriplesToCluster(randomVectors))

	startTime := time.Now()
	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "6", "euclidean"))

	dur := time.Since(startTime)
	// Easy way to check that the index was actually rebuilt
	require.Greater(t, dur, time.Second*4)
}

func TestVectorInQueryArgument(t *testing.T) {
	dropPredicate("vtest")
	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "euclidean"))

	numVectors := 100
	vectorSize := 4

	randomVectors, allVectors := generateRandomVectors(numVectors, vectorSize, "vtest")
	require.NoError(t, addTriplesToCluster(randomVectors))

	query := `query demo($v: float32vector) {
		 vector(func: similar_to(vtest, 1, $v)) {
				uid
		 }
	}`

	vectorString := fmt.Sprintf(`[%s]`, strings.Trim(strings.Join(strings.Fields(fmt.Sprint(allVectors[0])), ", "), "[]"))
	vars := map[string]string{
		"$v": vectorString,
	}

	_, err := processQueryWithVars(t, query, vars)
	require.NoError(t, err)
}

func TestVectorsMutateFixedLengthWithDiffrentIndexes(t *testing.T) {
	dropPredicate("vtest")

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "euclidean"))
	testVectorMutationSameLength(t)
	dropPredicate("vtest")

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "cosine"))
	testVectorMutationSameLength(t)
	dropPredicate("vtest")

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "dotproduct"))
	testVectorMutationSameLength(t)
	dropPredicate("vtest")
}

func TestVectorDeadlockwithTimeout(t *testing.T) {
	pred := "vtest1"
	dc = dgraphtest.NewComposeCluster()
	var cleanup func()
	client, cleanup, err := dc.Client()
	x.Panic(err)
	defer cleanup()

	for i := 0; i < 5; i++ {
		fmt.Println("Testing iteration: ", i)
		ctx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		err = client.LoginIntoNamespace(ctx, dgraphapi.DefaultUser,
			dgraphapi.DefaultPassword, x.RootNamespace)
		require.NoError(t, err)

		err = client.Alter(context.Background(), &api.Operation{
			DropAttr: pred,
		})
		dropPredicate(pred)
		setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidean"))
		numVectors := 10000
		vectorSize := 1000

		randomVectors, _ := generateRandomVectors(numVectors, vectorSize, pred)

		txn := client.NewTxn()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer func() { _ = txn.Discard(ctx) }()
		defer cancel()

		_, err = txn.Mutate(ctx, &api.Mutation{
			SetNquads: []byte(randomVectors),
			CommitNow: true,
		})
		require.Error(t, err)

		err = txn.Commit(ctx)
		require.Contains(t, err.Error(), "Transaction has already been committed or discarded")
	}
}

func TestVectorMutateDiffrentLengthWithDiffrentIndexes(t *testing.T) {
	dropPredicate("vtest")

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "euclidean"))
	testVectorMutationDiffrentLength(t, "can not compute euclidean distance on vectors of different lengths")
	dropPredicate("vtest")

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "cosine"))
	testVectorMutationDiffrentLength(t, "can not compute cosine distance on vectors of different lengths")
	dropPredicate("vtest")

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "dotproduct"))
	testVectorMutationDiffrentLength(t, "can not compute dot product on vectors of different lengths")
	dropPredicate("vtest")
}

func TestVectorReindex(t *testing.T) {
	dropPredicate("vtest")

	pred := "vtest"

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidean"))

	numVectors := 100
	vectorSize := 4

	randomVectors, allVectors := generateRandomVectors(numVectors, vectorSize, pred)
	require.NoError(t, addTriplesToCluster(randomVectors))

	setSchema(fmt.Sprintf(vectorSchemaWithoutIndex, pred))

	query := `{
		 vector(func: has(vtest)) {
				count(uid)
			 }
	 }`

	result := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[{"count":100}]}}`, result)

	triple := strings.Split(randomVectors, "\n")[0]
	_, err := querySingleVectorError(t, strings.Split(triple, `"`)[1], "vtest", false)
	require.NotNil(t, err)

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidean"))
	vector, err := querySingleVector(t, strings.Split(triple, `"`)[1], "vtest")
	require.NoError(t, err)
	require.Contains(t, allVectors, vector)
}

func TestVectorMutationWithoutIndex(t *testing.T) {
	dropPredicate("vtest")

	pred := "vtest"
	setSchema(fmt.Sprintf(vectorSchemaWithoutIndex, pred))

	numVectors := 1000
	vectorSize := 4

	randomVectors, _ := generateRandomVectors(numVectors, vectorSize, pred)
	require.NoError(t, addTriplesToCluster(randomVectors))

	query := `{
		 vector(func: has(vtest)) {
				count(uid)
			 }
	 }`

	result := processQueryNoErr(t, query)
	require.JSONEq(t, fmt.Sprintf(`{"data": {"vector":[{"count":%d}]}}`, numVectors), result)

	dropPredicate("vtest")

	pred = "vtest2"
	setSchema(fmt.Sprintf(vectorSchemaWithoutIndex, pred))

	randomVectors, _ = generateRandomVectors(numVectors, vectorSize, pred)
	require.NoError(t, addTriplesToCluster(randomVectors))

	query = `{
		 vector(func: has(vtest2)) {
				count(uid)
			 }
	 }`

	result = processQueryNoErr(t, query)
	require.JSONEq(t, fmt.Sprintf(`{"data": {"vector":[{"count":%d}]}}`, numVectors), result)
	dropPredicate("vtest2")
}

func TestVectorDelete(t *testing.T) {
	pred := "vtest"
	dropPredicate(pred)

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidean"))

	numVectors := 1000
	rdf, vectors := generateRandomVectors(numVectors, 10, "vtest")
	require.NoError(t, addTriplesToCluster(rdf))

	query := `{
		 vector(func: has(vtest)) {
				count(uid)
			 }
	 }`

	result := processQueryNoErr(t, query)
	require.JSONEq(t, fmt.Sprintf(`{"data": {"vector":[{"count":%d}]}}`, numVectors), result)

	allVectors, err := queryAllVectorsPred(t, "vtest")
	require.NoError(t, err)

	require.Equal(t, vectors, allVectors)

	triples := strings.Split(rdf, "\n")

	deleteTriple := func(idx int) string {
		triple := triples[idx]

		deleteTriplesInCluster(triple)
		uid := strings.Split(triple, " ")[0]
		query = fmt.Sprintf(`{
		  vector(func: uid(%s)) {
		   vtest
		  }
		}`, uid[1:len(uid)-1])

		result = processQueryNoErr(t, query)
		require.JSONEq(t, `{"data": {"vector":[]}}`, result)
		return triple

	}

	for i := 0; i < len(triples)-2; i++ {
		triple := deleteTriple(i)
		vector, err := querySingleVector(t, strings.Split(triple, `"`)[1], "vtest")
		require.NoError(t, err)
		require.Contains(t, allVectors, vector)
	}

	triple := deleteTriple(len(triples) - 2)
	// after deleteing all vectors, we should get an empty array of vectors in response when we do silimar_to query
	_, err = querySingleVectorError(t, strings.Split(triple, `"`)[1], "vtest", false)
	require.NoError(t, err)
}

func TestVectorUpdate(t *testing.T) {
	pred := "vtest"
	dropPredicate(pred)

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidean"))

	numVectors := 1000
	rdf, vectors := generateRandomVectors(1000, 10, "vtest")
	require.NoError(t, addTriplesToCluster(rdf))

	allVectors, err := queryAllVectorsPred(t, "vtest")
	require.NoError(t, err)

	require.Equal(t, vectors, allVectors)

	updateVectorQuery := func(idx int) {
		triple := strings.Split(rdf, "\n")[idx]
		updatedVec := updateVector(t, triple, "vtest")
		allVectors[idx] = updatedVec

		updatedVectors, err := queryMultipleVectorsUsingSimilarTo(t, allVectors[0], "vtest", 100)
		require.NoError(t, err)

		for _, i := range updatedVectors {
			require.Contains(t, allVectors, i)
		}
	}

	for i := 0; i < 1000; i++ {
		idx := rand.Intn(numVectors)
		updateVectorQuery(idx)
	}
}

func TestVectorWithoutQuote(t *testing.T) {
	pred := "test-ve"
	dropPredicate(pred)
	setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidean"))

	setJson := `
{
   "set": [
     {
       "test-ve": [1,0],
       "v-name":"ve1"
     },
     {
       "test-ve": [0.866025,0.5],
       "v-name":"ve2"
     },
     {
       "test-ve": [0.5,0.866025],
       "v-name":"ve3"
     },
     {
       "test-ve": [0,1],
       "v-name":"ve4"
     },
     {
       "test-ve": [-0.5,0.866025],
       "v-name":"ve5"
     },
     {
       "test-ve": [-0.866025,0.5],
       "v-name":"ve6"
     }
   ]
 }
	`
	txn1 := client.NewTxn()
	_, err := txn1.Mutate(context.Background(), &api.Mutation{
		SetJson: []byte(setJson),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Input for predicate \"test-ve\" of type vector is not vector")
}

func TestVectorTwoTxnWithoutCommit(t *testing.T) {
	pred := "vtest"
	dropPredicate(pred)

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidean"))

	rdf, vectors := generateRandomVectors(5, 5, "vtest")
	txn1 := client.NewTxn()
	_, err := txn1.Mutate(context.Background(), &api.Mutation{
		SetNquads: []byte(rdf),
	})
	require.NoError(t, err)

	rdf, _ = generateRandomVectors(5, 5, "vtest")
	txn2 := client.NewTxn()
	_, err = txn2.Mutate(context.Background(), &api.Mutation{
		SetNquads: []byte(rdf),
	})
	require.NoError(t, err)

	require.NoError(t, txn1.Commit(context.Background()))
	require.Error(t, txn2.Commit(context.Background()))
	resp, err := queryMultipleVectorsUsingSimilarTo(t, vectors[0], "vtest", 5)
	require.NoError(t, err)

	for i := 0; i < len(vectors); i++ {
		require.Contains(t, resp, vectors[i])
	}
}

func TestGetVector(t *testing.T) {
	setSchema("vectorNonIndex : float32vector .")

	rdfs := `
	<1> <vectorNonIndex> "[1.0, 1.0, 2.0, 2.0]" .
	<2> <vectorNonIndex> "[2.0, 1.0, 2.0, 2.0]" .`
	require.NoError(t, addTriplesToCluster(rdfs))

	query := `
		{
			me(func: has(vectorNonIndex)) {
				a as vectorNonIndex
			}
			aggregation() {
				avg(val(a))
				sum(val(a))
			}
		}
	`
	js := processQueryNoErr(t, query)
	k := `{
		"data": {
		  "me": [
			{
			  "vectorNonIndex": [
				1,
				1,
				2,
				2
			  ]
			},
			{
			  "vectorNonIndex": [
				2,
				1,
				2,
				2
			  ]
			}
		  ],
		  "aggregation": [
			{
			  "avg(val(a))": [
				1.5,
				1,
				2,
				2
			  ]
			},
			{
			  "sum(val(a))": [
				3,
				2,
				4,
				4
			  ]
			}
		  ]
		}
	  }`
	require.JSONEq(t, k, js)
}

func TestDotProductWithConstantVector(t *testing.T) {
	setSchema("vec452 : float32vector .")

	rdfs := `
		<1> <vec452> "[1.0, 1.0, 2.0, 2.0]" .
		<2> <vec452> "[2.0, 1.0, 2.0, 2.0]" .`
	require.NoError(t, addTriplesToCluster(rdfs))

	query := `query q($vec: float32vector) {
		q(func: has(vec452)) {
			v1 as vec452
			distance: Math(v1 dot $vec)
		}
	}`
	js, err := processQueryWithVars(t, query, map[string]string{"$vec": "[1.0, 1.0, 2.0, 2.0]"})
	require.NoError(t, err)
	k := `{"data":{"q":[{"vec452":[1,1,2,2],"distance":10},{"vec452":[2,1,2,2],"distance":11}]}}`
	require.JSONEq(t, k, js)

	query = `{
		q(func: has(vec452)) {
			v1 as vec452
			distance: Math(v1 dot v1)
		}
	}`
	require.JSONEq(t,
		`{"data":{"q":[{"vec452":[1,1,2,2],"distance":10},{"vec452":[2,1,2,2],"distance":13}]} }`,
		processQueryNoErr(t, query))
}
