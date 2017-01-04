package worker

import (
	"fmt"
	"time"

	"github.com/dgraph-io/dgraph/x"
)

func TempDebug() {
	time.Sleep(time.Second)
	fmt.Printf("~~~~~~~hi\n")
	n := groups().Node(0)
	for {
		lastIndex, err := n.store.LastIndex()
		doneUntil := n.applied.DoneUntil()
		x.Check(err)
		fmt.Printf("~~~~ %d %d\n", doneUntil, lastIndex)
		// doneUntil can be < lastIndex
		time.Sleep(100 * time.Millisecond)
	}
}
