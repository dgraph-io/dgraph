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

package dgraphapi

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"

	"github.com/dgraph-io/dgo/v240/protos/api"
)

func GenerateRandomVector(size int) []float32 {
	vector := make([]float32, size)
	for i := range size {
		vector[i] = rand.Float32() * 10
	}
	return vector
}

func formatVector(label string, vector []float32, index int) string {
	vectorString := fmt.Sprintf(`"[%s]"`, strings.Trim(strings.Join(strings.Fields(fmt.Sprint(vector)), ", "), "[]"))
	return fmt.Sprintf("<0x%x> <%s> %s . \n", index+10, label, vectorString)
}

func GenerateRandomVectors(lowerLimit, uppermLimit, vectorSize int, label string) (string, [][]float32) {
	var builder strings.Builder
	var vectors [][]float32
	// builder.WriteString("`")
	for i := lowerLimit; i < uppermLimit; i++ {
		randomVector := GenerateRandomVector(vectorSize)
		vectors = append(vectors, randomVector)
		formattedVector := formatVector(label, randomVector, i)
		builder.WriteString(formattedVector)
	}

	return builder.String(), vectors
}

func (gc *GrpcClient) QueryMultipleVectorsUsingSimilarTo(vector []float32, pred string, topK int) ([][]float32, error) {
	vectorQuery := fmt.Sprintf(`
	 {
		 vector(func: similar_to(%v, %v, "%v")) {
				uid
				%v
		  }
	 }`, pred, topK, vector, pred)
	resp, err := gc.Query(vectorQuery)

	if err != nil {
		return [][]float32{}, err
	}

	return UnmarshalVectorResp(resp)
}

func (gc *GrpcClient) QuerySingleVectorsUsingUid(uid, pred string) ([][]float32, error) {
	vectorQuery := fmt.Sprintf(`
	    {
		 vector(func: uid(%v)) {
			uid
			%v
		     }
	    }`, uid[1:len(uid)-1], pred)

	resp, err := gc.Query(vectorQuery)
	if err != nil {
		return [][]float32{}, err
	}

	return UnmarshalVectorResp(resp)
}

func UnmarshalVectorResp(resp *api.Response) ([][]float32, error) {
	type Data struct {
		Vector []struct {
			UID                 string    `json:"uid"`
			ProjectDescriptionV []float32 `json:"project_discription_v"`
		} `json:"vector"`
	}
	var data Data
	if err := json.Unmarshal(resp.Json, &data); err != nil {
		return nil, err
	}

	var vectors [][]float32
	for _, item := range data.Vector {
		vectors = append(vectors, item.ProjectDescriptionV)
	}
	return vectors, nil
}
