// +build linux

package main

import (
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/protos"
)

func TestPlugins(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	var soFiles []string
	for i, src := range []string{
		"./customtok/anagram/main.go",
		"./customtok/cidr/main.go",
		"./customtok/factor/main.go",
		"./customtok/rune/main.go",
	} {
		so := strconv.Itoa(i) + ".so"
		t.Logf("compiling plugin: src=%q so=%q", src, so)
		cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", so, src)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Could not compile plugin: %v", string(out))
		}
		absSO, err := filepath.Abs(so)
		check(t, err)
		defer os.Remove(absSO)
		soFiles = append(soFiles, absSO)
	}

	tmpDir, err := ioutil.TempDir("", "")
	check(t, err)
	defer os.RemoveAll(tmpDir)

	cluster := NewDgraphCluster(tmpDir)
	cluster.TokenizerPluginsArg = strings.Join(soFiles, ",")
	check(t, cluster.Start())
	defer cluster.Close()

	type testCase struct {
		query      string
		wantResult string
	}
	suite := func(initialSchema string, setJSON string, cases []testCase) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		check(t, cluster.client.Alter(ctx, &protos.Operation{
			DropAll: true,
		}))
		check(t, cluster.client.Alter(ctx, &protos.Operation{
			Schema: initialSchema,
		}))

		txn := cluster.client.NewTxn()
		_, err = txn.Mutate(ctx, &protos.Mutation{SetJson: []byte(setJSON)})
		check(t, err)
		check(t, txn.Commit(ctx))

		for _, test := range cases {
			txn := cluster.client.NewTxn()
			reply, err := txn.Query(ctx, test.query, nil)
			check(t, err)
			CompareJSON(t, test.wantResult, string(reply.GetJson()))
		}
	}

	suite(
		"word: string @index(anagram) .",
		//}},
		`[
			{ "word": "airmen" },
			{ "word": "marine" },
			{ "word": "beat" },
			{ "word": "beta" },
			{ "word": "race" },
			{ "word": "care" }
		]`,
		[]testCase{
			{`
				{ q(func: allof(word, anagram, "remain")) {
					word
				}}`, `
				{ "data": { "q": [
					{ "word": "airmen" },
					{ "word": "marine" }
				]}}`,
			},
			{`
				{ q(func: allof(word, anagram, "acre")) {
					word
				}}`, `
				{ "data": { "q": [
					{ "word": "race" },
					{ "word": "care" }
				]}}`,
			},
			{`
				{ q(func: allof(word, anagram, "beta")) {
					word
				}}`, `
				{ "data": { "q": [
					{ "word": "beta" },
					{ "word": "beat" }
				]}}`,
			},
		},
	)

	suite(
		"ip: string @index(cidr) .",
		`[
			{ "ip": "100.55.22.11/32" },
			{ "ip": "100.33.81.19/32" },
			{ "ip": "100.49.21.25/32" },
			{ "ip": "101.0.0.5/32" },
			{ "ip": "100.176.2.1/32" }
		]`,
		[]testCase{
			{`
				{ q(func: allof(ip, cidr, "100.48.0.0/12")) {
					ip
				}}`, `
				{ "data": { "q": [
					{ "ip": "100.55.22.11/32" },
					{ "ip": "100.49.21.25/32" }
				]}}`,
			},
			{`
				{ q(func: allof(ip, cidr, "100.32.0.0/11")) {
					ip
				}}`, `
				{ "data": { "q": [
					{ "ip": "100.55.22.11/32" },
					{ "ip": "100.33.81.19/32" },
					{ "ip": "100.49.21.25/32" }
				]}}`,
			},
			{`
				{ q(func: allof(ip, cidr, "100.0.0.0/8")) {
					ip
				}}`, `
				{ "data": { "q": [
					{ "ip": "100.55.22.11/32" },
					{ "ip": "100.33.81.19/32" },
					{ "ip": "100.49.21.25/32" },
					{ "ip": "100.176.2.1/32" }
				]}}`,
			},
			{`
				{ q(func: allof(ip, cidr, "101.0.0.0/8")) {
					ip
				}}`, `
				{ "data": { "q": [
					{ "ip": "101.0.0.5/32" }
				]}}`,
			},
		},
	)

	suite(
		"name: string @index(rune) .",
		`[
			{ "name": "Adam" },
			{ "name": "Aaron" },
			{ "name": "Amy" },
			{ "name": "Ronald" }
		]`,
		[]testCase{
			{`
				{ q(func: allof(name, rune, "An")) {
					name
				}}`, `
				{ "data": { "q": [
					{ "name": "Aaron" }
				]}}`,
			},
			{`
				{ q(func: allof(name, rune, "Am")) {
					name
				}}`, `
				{ "data": { "q": [
					{ "name": "Amy" },
					{ "name": "Adam" }
				]}}`,
			},
			{`
				{ q(func: allof(name, rune, "ron")) {
					name
				}}`, `
				{ "data": { "q": [
					{ "name": "Aaron" }
				]}}`,
			},
			{`
				{ q(func: allof(name, rune, "ron")) {
					name
				}}`, `
				{ "data": { "q": [
					{ "name": "Aaron" }
				]}}`,
			},
			{`
				{ q(func: allof(name, rune, "no")) {
					name
				}}`, `
				{ "data": { "q": [
					{ "name": "Ronald" },
					{ "name": "Aaron" }
				]}}`,
			},
			{`
				{ q(func: anyof(name, rune, "mr")) {
					name
				}}`, `
				{ "data": { "q": [
					{ "name": "Adam" },
					{ "name": "Aaron" },
					{ "name": "Amy" }
				]}}`,
			},
			{`
				{ q(func: anyof(name, rune, "da")) {
					name
				}}`, `
				{ "data": { "q": [
					{ "name": "Adam" },
					{ "name": "Aaron" },
					{ "name": "Ronald" }
				]}}`,
			},
		},
	)
	suite(
		"num: int @index(factor) .",
		`[
			{ "num": 2 },
			{ "num": 3 },
			{ "num": 4 },
			{ "num": 5 },
			{ "num": 6 },
			{ "num": 7 },
			{ "num": 8 },
			{ "num": 9 },
			{ "num": 10 },
			{ "num": 11 },
			{ "num": 12 },
			{ "num": 13 },
			{ "num": 14 },
			{ "num": 15 },
			{ "num": 16 },
			{ "num": 17 },
			{ "num": 18 },
			{ "num": 19 },
			{ "num": 20 },
			{ "num": 21 },
			{ "num": 22 },
			{ "num": 23 },
			{ "num": 24 },
			{ "num": 25 },
			{ "num": 26 },
			{ "num": 27 },
			{ "num": 28 },
			{ "num": 29 },
			{ "num": 30 }
		]`,
		[]testCase{
			{`
				{
					q(func: allof(num, factor, 5)) {
						num
					}
				}`, `
				{ "data": { "q": [
					{ "num": 15 },
					{ "num": 20 },
					{ "num": 5 },
					{ "num": 10 },
					{ "num": 30 },
					{ "num": 25 }
				]}}`,
			},
			{`
				{
					q(func: allof(num, factor, 15)) {
						num
					}
				}`, `
				{ "data": { "q": [
					{ "num": 15 },
					{ "num": 30 }
				]}}`,
			},
			{`
				{
					q(func: anyof(num, factor, 15)) {
						num
					}
				}`, `
				{ "data": { "q": [
					{ "num": 3 },
					{ "num": 5 },
					{ "num": 6 },
					{ "num": 9 },
					{ "num": 10 },
					{ "num": 12 },
					{ "num": 15 },
					{ "num": 18 },
					{ "num": 20 },
					{ "num": 21 },
					{ "num": 24 },
					{ "num": 25 },
					{ "num": 27 },
					{ "num": 30 }
				]}}`,
			},
		},
	)
}
