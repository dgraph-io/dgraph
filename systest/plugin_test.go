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
	"github.com/dgraph-io/dgraph/x"
)

func TestPlugins(t *testing.T) {
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
	suite := func(setupQuery string, cases []testCase) {

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		// TODO Dropall (doesn't work yet, wait for alter endpoint).
		/*
			dropAllMut := &protos.Mutation{
				DropAll: true,
			}
			txn := cluster.client.NewTxn()
			x.Check2(txn.Mutate(ctx, dropAllMut))
			x.Check(txn.Commit(ctx))
		*/

		// TODO: Set schema.
		addSchema := &protos.Mutation{}
		addSchema.Schema = []*protos.SchemaUpdate{
			&protos.SchemaUpdate{
				Predicate: "word",
				ValueType: uint32(protos.Posting_STRING),
				Directive: protos.SchemaUpdate_INDEX,
				Tokenizer: []string{"anagram"},
			},
		}
		txn := cluster.client.NewTxn()
		x.Check2(txn.Mutate(ctx, addSchema))
		x.Check(txn.Commit(ctx))

		// TODO: Set RDFs
		txn = cluster.client.NewTxn()
		x.Check2(txn.Mutate(ctx, &protos.Mutation{SetJson: []byte(`[
			{"word": "airmen"},
			{"word": "marine"},
			{"word": "beat"},
			{"word": "beta"},
			{"word": "race"},
			{"word": "care"}
		]`)}))
		x.Check(txn.Commit(ctx))

		// TODO: Run queries.
		txn = cluster.client.NewTxn()
		q := `
		{ q(func: allof(word, anagram, "remain")) {
			word
		}}`
		resp, err := txn.Query(ctx, q, nil)
		x.Check(err)
		want := `
		{ "data": { "q": [
			{ "word": "airmen" },
			{ "word": "marine" }
		]}}`
		CompareJSON(t, want, string(resp.GetJson()))
	}

	suite(`
		mutation{
			schema{
				word: string @index(anagram) .
			}
			set{
				_:1 <word> "airmen" .
				_:2 <word> "marine" .
				_:3 <word> "beat" .
				_:4 <word> "beta" .
				_:5 <word> "race" .
				_:6 <word> "care" .
			}
		}`,
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
			/*
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
			*/
		},
	)

	/*
		suite(`
			mutation{
				schema{
					ip: string @index(cidr) .
				}
				set{
					_:a <ip> "100.55.22.11/32" .
					_:b <ip> "100.33.81.19/32" .
					_:c <ip> "100.49.21.25/32" .
					_:d <ip> "101.0.0.5/32" .
					_:e <ip> "100.176.2.1/32" .
				}
			}`,
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
		suite(`
			mutation{
				schema{
					name: string @index(rune) .
				}
				set{
					_:ad <name> "Adam" .
					_:aa <name> "Aaron" .
					_:am <name> "Amy" .
					_:ro <name> "Ronald" .
				}
			}`,
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
		suite(`
			mutation{
				schema{
					num: int @index(factor) .
				}
				set{
					_:2 <num> "2"^^<xs:int> .
					_:3 <num> "3"^^<xs:int> .
					_:4 <num> "4"^^<xs:int> .
					_:5 <num> "5"^^<xs:int> .
					_:6 <num> "6"^^<xs:int> .
					_:7 <num> "7"^^<xs:int> .
					_:8 <num> "8"^^<xs:int> .
					_:9 <num> "9"^^<xs:int> .
					_:10 <num> "10"^^<xs:int> .
					_:11 <num> "11"^^<xs:int> .
					_:12 <num> "12"^^<xs:int> .
					_:13 <num> "13"^^<xs:int> .
					_:14 <num> "14"^^<xs:int> .
					_:15 <num> "15"^^<xs:int> .
					_:16 <num> "16"^^<xs:int> .
					_:17 <num> "17"^^<xs:int> .
					_:18 <num> "18"^^<xs:int> .
					_:19 <num> "19"^^<xs:int> .
					_:20 <num> "20"^^<xs:int> .
					_:21 <num> "21"^^<xs:int> .
					_:22 <num> "22"^^<xs:int> .
					_:23 <num> "23"^^<xs:int> .
					_:24 <num> "24"^^<xs:int> .
					_:25 <num> "25"^^<xs:int> .
					_:26 <num> "26"^^<xs:int> .
					_:27 <num> "27"^^<xs:int> .
					_:28 <num> "28"^^<xs:int> .
					_:29 <num> "29"^^<xs:int> .
					_:30 <num> "30"^^<xs:int> .
				}
			}`,
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
	*/
}
