package worker

import (
	"fmt"
	"time"

	_ "github.com/dgraph-io/dgraph/x"
)

func TempDebug() {
	time.Sleep(time.Second)
	fmt.Printf("~~~~~~~hi\n")
	//	n := groups().Node(0)
	for {
		//		lastIndex, err := n.store.LastIndex()
		//		doneUntil := n.applied.DoneUntil()
		//		x.Check(err)
		//		fmt.Printf("~~~~ %d %d\n", doneUntil, lastIndex)
		time.Sleep(100 * time.Millisecond)
	}
}
