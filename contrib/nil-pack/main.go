package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

// NOTES:
// numLists can be used to change the number of uids being updated.
// numeric uids are used. Get a uid lease before running the script.
// Assumes there's an alpha in port 9180 with no ACL enabled.
var numLists = 1000

const plistValue = "{\"type\":\"doc\",\"content\":[{\"type\":\"headings_block\",\"attrs\":{\"level\":2},\"content\":[{\"type\":\"text\",\"text\":\"Overview\"}]},{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"Are zoos a good thing or a bad thing? Students will read three different articles related to zoos, and they'll practice analyzing multiple authors' writing on the same topic.\"}]},{\"type\":\"headings_block\",\"attrs\":{\"level\":2},\"content\":[{\"type\":\"text\",\"text\":\"Activities\"}]},{\"type\":\"headings_block\",\"attrs\":{\"level\":3},\"content\":[{\"type\":\"text\",\"text\":\"Before Reading\"}]},{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"marks\":[{\"type\":\"bold\"}],\"text\":\"Activating Prior Knowledge: \"},{\"type\":\"text\",\"text\":\"Allow students to share their experiences at different zoos. Discuss their favorite memories of visiting zoos and the animals they enjoyed viewing.\u00a0\"}]},{\"type\":\"headings_block\",\"attrs\":{\"level\":3},\"content\":[{\"type\":\"text\",\"text\":\"PRO Assignment Reading Instructions\"}]},{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"marks\":[{\"type\":\"bold\"}],\"text\":\"Author's Claim\"},{\"type\":\"text\",\"text\":\": Highlight in YELLOW the claim the author is making, highlight in GREEN the evidence that supports the claim.\"}]},{\"type\":\"headings_block\",\"attrs\":{\"level\":3},\"content\":[{\"type\":\"text\",\"text\":\"After Reading\"}]},{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"marks\":[{\"type\":\"bold\"},{\"type\":\"link\",\"attrs\":{\"href\":\"https://media.newsela.com/article_media/extra/4.18.18.ComparingMultipleSources.pdf\",\"title\":null}}],\"text\":\"Comparing Multiple Sources\"},{\"type\":\"text\",\"marks\":[{\"type\":\"bold\"}],\"text\":\": \"},{\"type\":\"text\",\"text\":\"Students will complete the Comparing Multiple Sources graphic organizer to help them analyze multiple author's writing on the topic of animals in zoos.\"}]},{\"type\":\"headings_block\",\"attrs\":{\"level\":2},\"content\":[{\"type\":\"text\",\"text\":\"Scaffold\"}]},{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"marks\":[{\"type\":\"bold\"}],\"text\":\"Question Stems: \"},{\"type\":\"text\",\"text\":\"Use the following question stems to support students as they analyze multiple authors' writing:\"}]},{\"type\":\"list_block_0\",\"content\":[{\"type\":\"list_block_1\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"How do the texts differ? Why do you think this is?\"}]}]},{\"type\":\"list_block_1\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"\u00a0As a reader, which of the authors' approaches do you prefer? Why?\"}]}]},{\"type\":\"list_block_1\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"Why do you think this author chose to approach the topic this way?\"}]}]},{\"type\":\"list_block_1\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"What was the author's intention in his or her presentation of events?\"}]}]},{\"type\":\"list_block_1\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"Was the author's approach balanced? Why do you believe this?\"}]}]},{\"type\":\"list_block_1\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"Which source do you think was most reliable? Why?\"}]}]}]},{\"type\":\"headings_block\",\"attrs\":{\"level\":2},\"content\":[{\"type\":\"text\",\"text\":\"Extension\"}]},{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"marks\":[{\"type\":\"bold\"}],\"text\":\"Point of View Practice: \"},{\"type\":\"text\",\"text\":\"Write a short story from the point of view of an animal in a zoo. Explore the animals thoughts and feelings about being in captivity.\u00a0\"}]},{\"type\":\"headings_block\",\"attrs\":{\"level\":2},\"content\":[{\"type\":\"text\",\"text\":\"Standard\"}]},{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"marks\":[{\"type\":\"bold\"}],\"text\":\"RI.6.9:\"},{\"type\":\"text\",\"text\":\"\u00a0Compare/contrast two authors' presentations of same event\"}]}]}"

func retryQuery(dg *dgo.Dgraph, q string) (*api.Response, error) {
	for {
		resp, err := dg.NewTxn().Query(context.Background(), q)
		if err != nil && (strings.Contains(err.Error(), "Please retry") ||
			strings.Contains(err.Error(), "less than minTs")) {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		return resp, err
	}
}

func retryMutation(dg *dgo.Dgraph, mu *api.Mutation) error {
	for {
		_, err := dg.NewTxn().Mutate(context.Background(), mu)
		if err != nil && (strings.Contains(err.Error(), "Please retry") ||
			strings.Contains(err.Error(), "less than minTs")) {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		return err
	}
}

func sendMutations(client *dgo.Dgraph, wg *sync.WaitGroup) {
	for loop := 0; ; loop++ {
		for i := 1; i <= numLists; i++ {
			nquad := fmt.Sprintf("<%d> <pred> %q .", i, plistValue)
			// fmt.Printf("Writing value for uid %d\n", i)
			err := testutil.RetryMutation(client, &api.Mutation{
				CommitNow: true,
				SetNquads: []byte(nquad),
			})
			if err != nil {
				fmt.Printf("Error in mutation for list %d: %v\n", i, err)
			}
		}

		if loop == 0 {
			wg.Done()
		}
	}

}

type response struct {
	Q []map[string]string `json:"q"`
}

func sendQueries(client *dgo.Dgraph, wg *sync.WaitGroup) {
	wg.Wait()

	for {
		for i := 1; i <= numLists; i++ {
			// fmt.Printf("Querying value for uid %d\n", i)
			query := fmt.Sprintf("{ q(func: uid(%d)) { pred } }", i)

			res, err := retryQuery(client, query)
			if err != nil {
				fmt.Printf("Error querying mutation for list %d: %v\n", i, err)
				continue
			}

			r := &response{}
			if err := json.Unmarshal(res.Json, r); err != nil {
				fmt.Printf("Error unmarshalling response: %v\n", err)
			}

			if r.Q[0]["pred"] != plistValue {
				fmt.Printf("Expected %s Found %s\n", plistValue, r.Q[0]["pred"])
				os.Exit(1)
			}
		}
	}
}

func main() {
	client1, err := testutil.DgraphClient("localhost:9180")
	x.Check(err)
	client2, err := testutil.DgraphClient("localhost:9182")
	x.Check(err)
	client3, err := testutil.DgraphClient("localhost:9183")
	x.Check(err)

	clients := []*dgo.Dgraph{client1, client2, client3}
	wg := &sync.WaitGroup{}
	wg.Add(1)

	// Use this sync group to start queries only after all the mutations have been done at least
	// once.
	numMutations := 3
	mutationWg := &sync.WaitGroup{}
	mutationWg.Add(numMutations)
	// Send queries to all Alphas
	go sendQueries(client1, mutationWg)
	go sendQueries(client2, mutationWg)
	go sendQueries(client3, mutationWg)

	// Multiple mutations
	for i := 0; i < numMutations; i++ {
		go sendMutations(clients[i], mutationWg)
	}

	// This causes the script to wait indefinitely while queries and mutations run in
	// different goroutines.
	wg.Wait()
}
