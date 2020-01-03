package main

import (
	"fmt"
	"os"
	"testing"
)

func TestJepsen(t *testing.T) {
	s := os.Getenv("HOME") + "/go/src/github.com/dgraph-io/jepsen"
	jepsenRoot = &s
	jepsenUp()
	workloads := testAllWorkloads
	nemeses := testAllNemeses
	for _, w := range workloads {
		for _, n := range nemeses {
			t.Run(fmt.Sprintf("Workload=%v Nemesis=%v", w, n), func(t *testing.T) {
				status := runJepsenTest(&jepsenTest{
					workload:          w,
					nemesis:           n,
					timeLimit:         600,
					concurrency:       "6n",
					rebalanceInterval: "10h",
					nemesisInterval:   "10",
					localBinary:       "/gobin/dgraph",
					nodes:             "n1,n2,n3,n4,n5",
					skew:              "",
					testCount:         1,
				})
				if status != testPass {
					t.Errorf("Test failed.")
				}
			})
		}
	}
	jepsenDown()
}
