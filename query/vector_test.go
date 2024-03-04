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
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	vectorSchemaWithIndex = `%v: vfloat @index(hnsw(exponent: "%v", metric: "%v")) .`
)

const (
	vectorSchemaWithoutIndex = `%v: vfloat .`
)

func generateRandomVector(size int) []float64 {
	vector := make([]float64, size)
	for i := 0; i < size; i++ {
		vector[i] = rand.Float64() * 10 // Adjust the multiplier as needed
	}
	return vector
}

func formatVector(label string, vector []float64) string {
	vectorString := fmt.Sprintf(`"[%s]"`, strings.Trim(strings.Join(strings.Fields(fmt.Sprint(vector)), ", "), "[]"))
	return fmt.Sprintf("_:%x <%s> %s . \n", rand.Int(), label, vectorString)
}

func generateRandomVectors(numVectors, vectorSize int, label string) string {
	var builder strings.Builder

	// builder.WriteString("`")
	for i := 0; i < numVectors; i++ {
		randomVector := generateRandomVector(vectorSize)
		formattedVector := formatVector(label, randomVector)
		builder.WriteString(formattedVector)
		if i < numVectors-1 {
			builder.WriteString(" ")
		}
	}
	// builder.WriteString("`")

	return builder.String()
}

func testVectorMutationSameLength(t *testing.T) {
	rdf := `<0x1> <vtest> "[1.5, 2.0, 3.0, 4.5]" .
    <0x2> <vtest> "[0.0, -1.0, 2.5, 1.0]" .
    <0x3> <vtest> "[-2.0, 3.5, 1.0, -0.5]" .
    <0x4> <vtest> "[4.0, 2.0, -1.5, 0.0]" .
    <0x5> <vtest> "[-3.0, 1.5, 0.5, 2.0]" .
    <0x6> <vtest> "[0.5, -2.5, 1.0, 3.0]" .
    <0x7> <vtest> "[2.0, 0.0, -2.0, 1.5]" .
    <0x8> <vtest> "[1.0, 1.0, 1.0, 1.0]" .
    <0x9> <vtest> "[-1.0, -1.0, -1.0, -1.0]" .
    <0x10> <vtest> "[3.0, -3.0, 0.0, 2.5]" .`

	require.NoError(t, addTriplesToCluster(rdf))

	query := `{  
		        vector(func: similar_to(vtest, 1, "[1.5, 2.0, 3.0, 4.5]")) {
		               uid
			           vtest
		         }
		}`

	result := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[{"uid":"0x1","vtest":[1.5, 2.0, 3.0, 4.5]}]}}`, result)

	query = `{  
			vector(func: similar_to(vtest, 1, "[-3.0, 1.5, 0.5, 2.0]")) {
				   uid
				   vtest
			 }
	}`

	result = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[{"uid":"0x5","vtest":[-3.0, 1.5, 0.5, 2.0]}]}}`, result)

	query = `{  
			vector(func: similar_to(vtest, 1, "[1.0, 1.0, 1.0, 1.0]")) {
				   uid
				   vtest
			 }
	}`
	result = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[{"uid":"0x8","vtest":[1.0, 1.0, 1.0, 1.0]}]}}`, result)

	query = `{  
		vector(func: similar_to(vtest, 1, "[3.0, -3.0, 0.0, 2.5]")) {
			   uid
			   vtest
		     }
    }`
	result = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[{"uid":"0x10","vtest":[3.0, -3.0, 0.0, 2.5]}]}}`, result)

	query = `{
		vector(func: uid(9)) {
		 uid
		 vtest
		}
	 }`

	result = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[{"uid":"0x9","vtest":[-1.0, -1.0, -1.0, -1.0]}]}}`, result)

	query = `{
		vector(func: uid(7)) {
		 uid
		 vtest
		}
	 }`

	result = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[{"uid":"0x7","vtest":[2.0, 0.0, -2.0, 1.5]}]}}`, result)
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

func TestMutateVectorsFixedLengthWithDiffrentIndexes(t *testing.T) {
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

func TestMutateVectorsDiffrentLengthWithDiffrentIndexes(t *testing.T) {
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

func TestVectorMutationWithoutIndex(t *testing.T) {
	dropPredicate("vtest")

	pred := "vtest"
	setSchema(fmt.Sprintf(vectorSchemaWithoutIndex, pred))

	numVectors := 100
	vectorSize := 4

	randomVectors := generateRandomVectors(numVectors, vectorSize, pred)
	require.NoError(t, addTriplesToCluster(randomVectors))

	query := `{  
		vector(func: has(vtest)) {
			   count(uid)
		    }
    }`

	result := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[{"count":100}]}}`, result)

	dropPredicate("vtest")

	pred = "vtest2"
	setSchema(fmt.Sprintf(vectorSchemaWithoutIndex, pred))

	randomVectors = generateRandomVectors(numVectors, vectorSize, pred)
	require.NoError(t, addTriplesToCluster(randomVectors))

	query = `{  
		vector(func: has(vtest2)) {
			   count(uid)
		    }
    }`

	result = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[{"count":100}]}}`, result)
	dropPredicate("vtest2")
}

func TestDeleteVector(t *testing.T) {
	pred := "vtest"

	dropPredicate(pred)

	setSchema(fmt.Sprintf(vectorSchemaWithIndex, pred, "4", "euclidian"))

	rdf := `<0x21> <vtest> "[2.5, 3.0, 4.5, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.5]" .
	<0x22> <vtest> "[1.0, 2.0, -1.5, 0.5, -2.0, 3.0, -4.5, 0.0, 2.5, 1.0]" .
	<0x23> <vtest> "[0.0, -1.5, 2.0, -3.5, 4.0, 1.5, 0.0, -2.0, 2.5, -1.0]" .
	<0x24> <vtest> "[-4.0, 1.5, 0.0, -2.5, 3.0, -2.5, 1.0, 2.5, 0.5, -1.0]" .
	<0x25> <vtest> "[3.0, -2.5, 1.0, 0.0, -4.0, 2.5, -3.0, -1.0, 1.0, 0.0]" .
	<0x31> <vtest> "[1.0, 2.0, 3.5, 4.0, 5.5, 6.0, 7.5, 8.0, 9.5, 10.0]" .
	<0x32> <vtest> "[2.0, -1.0, 2.5, 1.0, -3.0, 2.0, -1.5, 0.0, 1.5, -2.5]" .
	<0x33> <vtest> "[3.0, 2.5, -1.0, 0.5, 2.0, -1.0, 0.0, 1.0, -1.5, 2.0]" .
	<0x34> <vtest> "[-4.0, -2.0, 1.5, 0.0, 3.0, 2.5, -1.0, 2.5, 0.5, -1.0]" .
	<0x35> <vtest> "[-3.0, 1.5, 0.5, 2.0, -4.0, 2.5, -3.0, -1.0, 1.0, 0.0]" . `

	require.NoError(t, addTriplesToCluster(rdf))

	query := `{  
		vector(func: has(vtest)) {
			   count(uid)
		    }
	}`

	result := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[{"count":10}]}}`, result)

	deleteTriplesInCluster(`<0x23> <vtest> "[0.0, -1.5, 2.0, -3.5, 4.0, 1.5, 0.0, -2.0, 2.5, -1.0]" .`)

	query = `{
		vector(func: uid(23)) {
		 vtest
		}
	 }`

	result = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[]}}`, result)

	query = `{
		vector(func: similar_to(vtest, 1, "[0.0, -1.5, 2.0, -3.5, 4.0, 1.5, 0.0, -2.0, 2.5, -1.0]")) {
		 uid
		 vtest
		 }
	 }`

	result = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[]}}`, result)

	deleteTriplesInCluster(`<0x32> <vtest> "[2.0, -1.0, 2.5, 1.0, -3.0, 2.0, -1.5, 0.0, 1.5, -2.5]" .`)

	query = `{
		vector(func: similar_to(vtest, 1, "[2.0, -1.0, 2.5, 1.0, -3.0, 2.0, -1.5, 0.0, 1.5, -2.5]")) {
		 uid
		 vtest
		 }
	 }`

	result = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[]}}`, result)
	query = `{
		vector(func: uid(32)) {
		 vtest
		}
	 }`

	result = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[]}}`, result)

	deleteTriplesInCluster(`<0x35> <vtest> "[-3.0, 1.5, 0.5, 2.0, -4.0, 2.5, -3.0, -1.0, 1.0, 0.0]" . `)

	query = `{
		vector(func: uid(35)) {
		 vtest
		}
	 }`

	result = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[]}}`, result)

	deleteTriplesInCluster(`<0x35> <vtest> "[-4.0, 1.5, 0.0, -2.5, 3.0, -2.5, 1.0, 2.5, 0.5, -1.0]" . `)
	query = `{
		vector(func: similar_to(vtest, 1, "[-4.0, 1.5, 0.0, -2.5, 3.0, -2.5, 1.0, 2.5, 0.5, -1.0]")) {
		 uid
		 vtest
		 }
	 }`

	result = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"vector":[]}}`, result)
}
