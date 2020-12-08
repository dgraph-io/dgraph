package live

import (
	"github.com/dgraph-io/dgraph/systest/bulk_live/common"
	"testing"
)

func TestLiveCases(t *testing.T) {
	t.Run("live test cases", common.RunLiveCases)
}
