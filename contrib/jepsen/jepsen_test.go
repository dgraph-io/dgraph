package main

import (
	"testing"
)

// These tests use the same flags defined in main.go.

func test(t *testing.T, workload, nemesis string) {
	jepsenUp()
	defer jepsenDown()
	err := runJepsenTest(&jepsenTest{
		workload:          workload,
		nemesis:           nemesis,
		timeLimit:         *timeLimit,
		concurrency:       *concurrency,
		rebalanceInterval: *rebalanceInterval,
		nemesisInterval:   *nemesisInterval,
		localBinary:       *localBinary,
		nodes:             *nodes,
		replicas:          *replicas,
		skew:              *skew,
		testCount:         *testCount,
		deferDbTeardown:   *deferDbTeardown,
	})
	if err == errTestFail {
		t.Error("Test failed.")
	} else if err == errTestIncomplete {
		t.Skip("Test incomplete.")
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
