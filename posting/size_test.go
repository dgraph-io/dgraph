package posting

import (
	"fmt"
	"testing"

	"github.com/dgraph-io/dgraph/protos/pb"
)

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
var a *List

var c *pb.PostingList

var d *pb.UidPack

var e *pb.UidBlock

func TestYo(t *testing.T) {
	fmt.Println("SD")
	a = &List{}
	a.mutationMap = make(map[uint64]*pb.PostingList)
	fmt.Println("SD3245")
	a.mutationMap[1] = &pb.PostingList{}
	fmt.Println(a.mutationMap[1])
}
func BenchmarkPosting(b *testing.B) {
	for i := 0; i < b.N; i++ {
		a = &List{}
		a.mutationMap = make(map[uint64]*pb.PostingList)
		a.mutationMap[1] = &pb.PostingList{}
		a.mutationMap[2] = &pb.PostingList{}
		fmt.Println(a.DeepSize())
	}
}

func BenchmarkPostingList(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c = &pb.PostingList{}
	}
}

func BenchmarkUidPack(b *testing.B) {
	for i := 0; i < b.N; i++ {
		d = &pb.UidPack{}
	}
}

func BenchmarkUidBlock(b *testing.B) {
	for i := 0; i < b.N; i++ {
		e = &pb.UidBlock{}
	}
}
