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
// GNU Lesser General Public License for more detailg.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package network

import (
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/lib/utils"
)

// list of IPFS peers, for testing only
var TestPeers = []string{
	"/ip4/104.131.131.82/tcp/4001/ipfs/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
	"/ip4/104.236.179.241/tcp/4001/ipfs/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
	"/ip4/128.199.219.111/tcp/4001/ipfs/QmSoLSafTMBsPKadTEgaXctDQVcqN88CNLHXMkTNwMKPnu",
	"/ip4/104.236.76.40/tcp/4001/ipfs/QmSoLV4Bbm51jM9C4gDYZQ9Cy3U6aXMJDAbzgu2fzaDs64",
	"/ip4/178.62.158.247/tcp/4001/ipfs/QmSoLer265NRgSp2LA3dPaeykiS1J6DifTC88f5uVQKNAd",
	"/ip6/2604:a880:1:20::203:d001/tcp/4001/ipfs/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
	"/ip6/2400:6180:0:d0::151:6001/tcp/4001/ipfs/QmSoLSafTMBsPKadTEgaXctDQVcqN88CNLHXMkTNwMKPnu",
	"/ip6/2604:a880:800:10::4a:5001/tcp/4001/ipfs/QmSoLV4Bbm51jM9C4gDYZQ9Cy3U6aXMJDAbzgu2fzaDs64",
	"/ip6/2a03:b0c0:0:1010::23:1001/tcp/4001/ipfs/QmSoLer265NRgSp2LA3dPaeykiS1J6DifTC88f5uVQKNAd",
}

func TestStringToAddrInfo(t *testing.T) {
	for _, str := range TestPeers {
		pi, err := stringToAddrInfo(str)
		if err != nil {
			t.Fatal(err)
		}

		if pi.ID.Pretty() != str[len(str)-46:] {
			t.Errorf("got %s expected %s", pi.ID.Pretty(), str)
		}
	}
}

func TestStringsToAddrInfos(t *testing.T) {
	pi, err := stringsToAddrInfos(TestPeers)
	if err != nil {
		t.Fatal(err)
	}
	for k, pi := range pi {
		if pi.ID.Pretty() != TestPeers[k][len(TestPeers[k])-46:] {
			t.Errorf("got %s expected %s", pi.ID.Pretty(), TestPeers[k])
		}
	}
}

func TestGenerateKey(t *testing.T) {
	testDir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	keyA, err := generateKey(0, testDir)
	if err != nil {
		t.Fatal(err)
	}

	keyB, err := generateKey(0, testDir)
	if err != nil {
		t.Fatal(err)
	}

	if reflect.DeepEqual(keyA, keyB) {
		t.Error("Generated keys should not match")
	}

	keyC, err := generateKey(1, testDir)
	if err != nil {
		t.Fatal(err)
	}

	keyD, err := generateKey(1, testDir)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(keyC, keyD) {
		t.Error("Generated keys should match")
	}
}
