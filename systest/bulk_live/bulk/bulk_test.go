package main

import (
	"github.com/dgraph-io/dgraph/systest/bulk_live/common"
	"testing"
)

// run this in sequential order. cleanup is necessary for bulk loader to work
func TestBulkCases(t *testing.T) {
	t.Run("bulk test cases", common.RunBulkCases)
}



