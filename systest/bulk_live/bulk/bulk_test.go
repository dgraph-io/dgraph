package main

import (
	"github.com/dgraph-io/dgraph/systest/bulk_live/common"
	"testing"
)

func TestBulkCases(t *testing.T) {
	t.Run("bulk test cases", common.RunBulkCases)
}



