package main

import (
	"flag"
	"os"
	"path"
	"testing"

	"github.com/dgraph-io/dgraph/contrib/jepsen/browser"
)

var (
	optJepsenRoot = flag.String("jepsen-root", "",
		"Directory path to jepsen repo. This sets the JEPSEN_ROOT env var for Jepsen ./up.sh.")
	optTimeLimit = flag.Int("time-limit", 600,
		"Time limit per Jepsen test in seconds.")
	optConcurrency = flag.String("concurrency", "6n",
		"Number of concurrent workers per test. \"6n\" means 6 workers per node.")
	optRebalanceInterval = flag.String("rebalance-interval", "10h",
		"Interval of Dgraph's tablet rebalancing.")
	optNemesisInterval = flag.String("nemesis-interval", "10",
		"Roughly how long to wait (in seconds) between nemesis operations.")
	optLocalBinary = flag.String("local-binary", "/gobin/dgraph",
		"Path to Dgraph binary within the Jepsen control node.")
	optNodes     = flag.String("nodes", "n1,n2,n3,n4,n5", "Nodes to run on.")
	optReplicas  = flag.Int("replicas", 3, "How many replicas of data should dgraph store?")
	optSkew      = flag.String("skew", "", "Skew clock amount. (tiny, small, big, huge)")
	optTestCount = flag.Int("test-count", 1, "Test count per Jepsen test.")
	optJaeger    = flag.String("jaeger", "http://jaeger:14268",
		"Run with Jaeger collector. Set to empty string to disable collection to Jaeger.")
	optJaegerSaveTraces = flag.Bool("jaeger-save-traces", true, "Save Jaeger traces on test error.")
	optDeferDbTeardown  = flag.Bool("defer-db-teardown", false,
		"Wait until user input to tear down DB nodes")
	optWeb = flag.Bool("web", true, "Open the test results page in the browser.")
)

func buildDgraph(t *testing.T) {
	t.Helper()
	exPath, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmd := command("make", "install")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	p := path.Join(exPath, "..", "..")
	t.Log(p)
	cmd.Dir = p
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}

func test(t *testing.T, workload, nemesis string) {
	buildDgraph(t)
	jepsenUp(*optJepsenRoot)
	defer jepsenDown(*optJepsenRoot)
	if err := jepsenServe(); err != nil {
		t.Fatal(err)
	}
	shouldOpenPage := *optWeb
	if shouldOpenPage {
		url := jepsenURL()
		browser.Open(url)
		if *jaeger != "" {
			browser.Open("http://localhost:16686")
		}
	}

	err := runJepsenTest(&jepsenTest{
		workload:          workload,
		nemesis:           nemesis,
		timeLimit:         *optTimeLimit,
		concurrency:       *optConcurrency,
		rebalanceInterval: *optRebalanceInterval,
		nemesisInterval:   *optNemesisInterval,
		localBinary:       *optLocalBinary,
		nodes:             *optNodes,
		replicas:          *optReplicas,
		skew:              *optSkew,
		testCount:         *optTestCount,
		deferDbTeardown:   *optDeferDbTeardown,
	})
	if err != nil {
		t.Log(err.Error())
	} else {
		t.Log("No error")
	}
	if err == errTestFail {
		if *optJaegerSaveTraces {
			saveJaegerTracesToJepsen(*optJepsenRoot)
		}
		t.Error(err.Error())
	} else if err == errTestIncomplete {
		t.Error(err.Error())
	}
}

func TestJepsenBankNone(t *testing.T) {
	test(t, "bank", "none")
}
func TestJepsenBankPartitionRing(t *testing.T) {
	test(t, "bank", "partition-ring")
}
func TestJepsenBankKillAlphaKillZero(t *testing.T) {
	test(t, "bank", "kill-alpha,kill-zero")
}
func TestJepsenBankMoveTablet(t *testing.T) {
	test(t, "bank", "move-tablet")
}
func TestJepsenDeleteNone(t *testing.T) {
	test(t, "delete", "none")
}
func TestJepsenDeletePartitionRing(t *testing.T) {
	test(t, "delete", "partition-ring")
}
func TestJepsenDeleteKillAlphaKillZero(t *testing.T) {
	test(t, "delete", "kill-alpha,kill-zero")
}
func TestJepsenDeleteMoveTablet(t *testing.T) {
	test(t, "delete", "move-tablet")
}
func TestJepsenLongForkNone(t *testing.T) {
	test(t, "long-fork", "none")
}
func TestJepsenLongForkPartitionRing(t *testing.T) {
	test(t, "long-fork", "partition-ring")
}
func TestJepsenLongForkKillAlphaKillZero(t *testing.T) {
	test(t, "long-fork", "kill-alpha,kill-zero")
}
func TestJepsenLongForkMoveTablet(t *testing.T) {
	test(t, "long-fork", "move-tablet")
}
func TestJepsenLinearizableRegisterNone(t *testing.T) {
	test(t, "linearizable-register", "none")
}
func TestJepsenLinearizableRegisterPartitionRing(t *testing.T) {
	test(t, "linearizable-register", "partition-ring")
}
func TestJepsenLinearizableRegisterKillAlphaKillZero(t *testing.T) {
	test(t, "linearizable-register", "kill-alpha,kill-zero")
}
func TestJepsenLinearizableRegisterMoveTablet(t *testing.T) {
	test(t, "bank", "move-tablet")
}
func TestJepsenUidLinearizableRegisterNone(t *testing.T) {
	test(t, "uid-linearizable-register", "none")
}
func TestJepsenUidLinearizableRegisterPartitionRing(t *testing.T) {
	test(t, "uid-linearizable-register", "partition-ring")
}
func TestJepsenUidLinearizableRegisterKillAlphaKillZero(t *testing.T) {
	test(t, "uid-linearizable-register", "kill-alpha,kill-zero")
}
func TestJepsenUidLinearizableRegisterMoveTablet(t *testing.T) {
	test(t, "uid-linearizable-register", "move-tablet")
}
func TestJepsenUpsertNone(t *testing.T) {
	test(t, "upsert", "none")
}
func TestJepsenUpsertPartitionRing(t *testing.T) {
	test(t, "upsert", "partition-ring")
}
func TestJepsenUpsertKillAlphaKillZero(t *testing.T) {
	test(t, "upsert", "kill-alpha,kill-zero")
}
func TestJepsenUpsertMoveTablet(t *testing.T) {
	test(t, "upsert", "move-tablet")
}
func TestJepsenSetNone(t *testing.T) {
	test(t, "set", "none")
}
func TestJepsenSetPartitionRing(t *testing.T) {
	test(t, "set", "partition-ring")
}
func TestJepsenSetKillAlphaKillZero(t *testing.T) {
	test(t, "set", "kill-alpha,kill-zero")
}
func TestJepsenSetMoveTablet(t *testing.T) {
	test(t, "set", "move-tablet")
}
func TestJepsenUidSetNone(t *testing.T) {
	test(t, "uid-set", "none")
}
func TestJepsenUidSetPartitionRing(t *testing.T) {
	test(t, "uid-set", "partition-ring")
}
func TestJepsenUidSetKillAlphaKillZero(t *testing.T) {
	test(t, "uid-set", "kill-alpha,kill-zero")
}
func TestJepsenUidSetMoveTablet(t *testing.T) {
	test(t, "uid-set", "move-tablet")
}
func TestJepsenSequentialNone(t *testing.T) {
	test(t, "sequential", "none")
}
func TestJepsenSequentialPartitionRing(t *testing.T) {
	test(t, "sequential", "partition-ring")
}
func TestJepsenSequentialKillAlphaKillZero(t *testing.T) {
	test(t, "sequential", "kill-alpha,kill-zero")
}
func TestJepsenSequentialMoveTablet(t *testing.T) {
	test(t, "sequential", "move-tablet")
}

func TestMain(m *testing.M) {
	flag.Parse()
	jepsenRoot = optJepsenRoot
	os.Exit(m.Run())
}
