package live

import (
	"github.com/dgraph-io/dgraph/systest/bulk_live/common"
	"testing"
)

// run this in sequential order. cleanup is necessary for bulk loader to work
func TestLiveCases(t *testing.T) {
	t.Run("live test cases", common.RunLiveCases)
}
