package main

import (
	"fmt"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/x"
)

func dfs(node *gql.FilterTree, depth int) {
	if node == nil {
		x.Printf("depth %d: nil", depth)
		return
	}
	x.Printf("depth %d: %v\n", depth, node)
	for _, c := range node.Child {
		dfs(c, depth+1)
	}
}

func main() {
	x.Init()

	query := `
	query {
		me(_uid_:0x0a) {
			friends @filter(  equal("type.object.name.en","john") &&  
			   equal() &&
				   what("haha")     ) {
				name
			}
			gender @filter(),age @filter(what())
			hometown
		}
	}
`
	gq, _, err := gql.Parse(query)
	x.Check(err)

	fmt.Println(gq)

	x.Check(err)
	x.Assert(gq != nil)
	x.Assertf(len(gq.Children) == 4, "Expected 4 children. Got: %v", len(gq.Children))

	x.Check(checkAttr(gq.Children[0], "friends"))
	x.Check(checkAttr(gq.Children[1], "gender"))
	x.Check(checkAttr(gq.Children[2], "age"))
	x.Check(checkAttr(gq.Children[3], "hometown"))

	x.Printf("\nfriends filter:\n")
	dfs(gq.Children[0].Filter, 0)

	x.Printf("\ngender filter:\n")
	dfs(gq.Children[1].Filter, 0)

	x.Printf("\nage filter:\n")
	dfs(gq.Children[2].Filter, 0)

	x.Printf("\nhometown filter:\n")
	dfs(gq.Children[3].Filter, 0)

}

//query {
//		me(_uid_:0x0a) {
//			friends {
//				name
//			}
//			gender,age
//			hometown
//		}
//	}

func checkAttr(g *gql.GraphQuery, attr string) error {
	if g.Attr != attr {
		return fmt.Errorf("Expected attr: %v. Got: %v", attr, g.Attr)
	}
	return nil
}
