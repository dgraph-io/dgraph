//go:build integration

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

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/stretchr/testify/require"
)

var (
	vectorSchemaWithIndex = `%v: float32vector @index(hnsw(exponent: "%v", metric: "%v")) .`
)

const (
	vectorSchemaWithoutIndex = `%v: float32vector .`
)

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

func TestVectorsMutateFixedLengthWithDiffrentIndexes(t *testing.T) {
	dropPredicate("vtest")

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "euclidian"))
	testVectorMutationSameLength(t)
	dropPredicate("vtest")

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "cosine"))
	testVectorMutationSameLength(t)
	dropPredicate("vtest")

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "dot_product"))
	testVectorMutationSameLength(t)
	dropPredicate("vtest")
}

func TestVectorMutateDiffrentLengthWithDiffrentIndexes(t *testing.T) {
	dropPredicate("vtest")

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "euclidian"))
	testVectorMutationDiffrentLength(t, "can not subtract vectors of different lengths")
	dropPredicate("vtest")

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "cosine"))
	testVectorMutationDiffrentLength(t, "can not compute dot product on vectors of different lengths")
	dropPredicate("vtest")

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, "vtest", "4", "dot_product"))
	testVectorMutationDiffrentLength(t, "can not subtract vectors of different lengths")
	dropPredicate("vtest")
}

func TestVectorReindex(t *testing.T) {
	dropPredicate("vtest")

	pred := "vtest"

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidian"))

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

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidian"))
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

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidian"))

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
	_, err = querySingleVectorError(t, strings.Split(triple, `"`)[1], "vtest", false)
	require.NotNil(t, err)
}

func TestVectorUpdate(t *testing.T) {
	pred := "vtest"
	dropPredicate(pred)

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidian"))

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

func TestVectorTwoTxnWithoutCommit(t *testing.T) {
	pred := "vtest"
	dropPredicate(pred)

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidian"))

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
