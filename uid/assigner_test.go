/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package uid

import (
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

func TestAssignNew(t *testing.T) {
	type args struct {
		N     int
		group uint32
	}
	tests := []struct {
		name string
		args args
		want *taskp.Mutations
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AssignNew(tt.args.N, tt.args.group); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AssignNew() = %v, want %v", got, tt.want)
			}
		})
	}
}

func init() {
	x.SetTestRun()
	x.Init()

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)

	ps, err := store.NewStore(dir)
	x.Check(err)

	posting.Init(ps)

	group.ParseGroupConfig("")
}

func BenchmarkAssignNew(b *testing.B) {
	Init()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AssignNew(1000, 1)
	}
}

func BenchmarkAssignNewParallel(b *testing.B) {
	Init()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			AssignNew(1000, 1)
		}
	})

}
