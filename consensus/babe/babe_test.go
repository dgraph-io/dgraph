// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package babe

import (
	"math"
	"math/big"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/runtime"
	"github.com/ChainSafe/gossamer/trie"
)

const POLKADOT_RUNTIME_FP string = "../../polkadot_runtime.wasm"

func newRuntime(t *testing.T) *runtime.Runtime {
	fp, err := filepath.Abs(POLKADOT_RUNTIME_FP)
	if err != nil {
		t.Fatal("could not create filepath")
	}

	tt := &trie.Trie{}

	r, err := runtime.NewRuntime(fp, tt)
	if err != nil {
		t.Fatal(err)
	} else if r == nil {
		t.Fatal("did not create new VM")
	}

	return r
}

func TestCalculateThreshold(t *testing.T) {
	// C = 1
	var C1 uint64 = 1
	var C2 uint64 = 1
	var authorityIndex uint64 = 0
	authorityWeights := []uint64{1, 1, 1}

	expected := new(big.Int).Lsh(big.NewInt(1), 128)

	threshold, err := calculateThreshold(C1, C2, authorityIndex, authorityWeights)
	if err != nil {
		t.Fatal(err)
	}

	if threshold.Cmp(expected) != 0 {
		t.Fatalf("Fail: got %d expected %d", threshold, expected)
	}

	// C = 1/2
	C2 = 2

	theta := float64(1) / float64(3)
	c := float64(C1) / float64(C2)
	pp := 1 - c
	pp_exp := math.Pow(pp, theta)
	p := 1 - pp_exp
	p_rat := new(big.Rat).SetFloat64(p)
	q := new(big.Int).Lsh(big.NewInt(1), 128)
	expected = q.Mul(q, p_rat.Num()).Div(q, p_rat.Denom())

	threshold, err = calculateThreshold(C1, C2, authorityIndex, authorityWeights)
	if err != nil {
		t.Fatal(err)
	}

	if threshold.Cmp(expected) != 0 {
		t.Fatalf("Fail: got %d expected %d", threshold, expected)
	}
}

func TestCalculateThreshold_AuthorityWeights(t *testing.T) {
	var C1 uint64 = 5
	var C2 uint64 = 17
	var authorityIndex uint64 = 3
	authorityWeights := []uint64{3, 1, 4, 6, 10}

	theta := float64(6) / float64(24)
	c := float64(C1) / float64(C2)
	pp := 1 - c
	pp_exp := math.Pow(pp, theta)
	p := 1 - pp_exp
	p_rat := new(big.Rat).SetFloat64(p)
	q := new(big.Int).Lsh(big.NewInt(1), 128)
	expected := q.Mul(q, p_rat.Num()).Div(q, p_rat.Denom())

	threshold, err := calculateThreshold(C1, C2, authorityIndex, authorityWeights)
	if err != nil {
		t.Fatal(err)
	}

	if threshold.Cmp(expected) != 0 {
		t.Fatalf("Fail: got %d expected %d", threshold, expected)
	}
}

func TestRunLottery(t *testing.T) {
	rt := newRuntime(t)
	babesession := NewSession([32]byte{}, [64]byte{}, rt)
	babesession.authorityIndex = 0
	babesession.authorityWeights = []uint64{1, 1, 1}
	conf := &BabeConfiguration{
		SlotDuration:         6000,
		C1:                   1,
		C2:                   4,
		MedianRequiredBlocks: 1000,
	}

	epoch := &Epoch{
		EpochIndex:     0,
		StartSlot:      0,
		Duration:       2400,
		Authorities:    [32]byte{},
		Randomness:     0,
		SecondarySlots: false,
	}

	babesession.config = conf
	babesession.epochData = epoch

	_, err := babesession.runLottery(0)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCalculateThreshold_Failing(t *testing.T) {
	var C1 uint64 = 5
	var C2 uint64 = 4
	var authorityIndex uint64 = 3
	authorityWeights := []uint64{3, 1, 4, 6, 10}

	_, err := calculateThreshold(C1, C2, authorityIndex, authorityWeights)
	if err == nil {
		t.Fatal("Fail: did not err for c>1")
	}
}

func TestStartupData(t *testing.T) {
	rt := newRuntime(t)
	babesession := NewSession([32]byte{}, [64]byte{}, rt)
	res, err := babesession.startupData()
	if err != nil {
		t.Fatal(err)
	}

	expected := &BabeConfiguration{
		SlotDuration:         6000,
		C1:                   1,
		C2:                   4,
		MedianRequiredBlocks: 1000,
	}

	if !reflect.DeepEqual(res, expected) {
		t.Errorf("Fail: got %v expected %v\n", res, expected)
	}
}

func TestEpoch(t *testing.T) {
	rt := newRuntime(t)
	babesession := NewSession([32]byte{}, [64]byte{}, rt)
	res, err := babesession.epoch()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(res)

	expected := &Epoch{
		EpochIndex:     0,
		StartSlot:      0,
		Duration:       2400,
		Authorities:    [32]byte{},
		Randomness:     0,
		SecondarySlots: false,
	}

	if !reflect.DeepEqual(res, expected) {
		t.Errorf("Fail: got %v expected %v\n", res, expected)
	}
}
