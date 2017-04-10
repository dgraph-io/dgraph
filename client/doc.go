/*
Package client provides helper function for interacting with the Dgraph server.
You can use it to run mutations and queries. You can also use BatchMutation
to upload data concurrently. It communicates with the server using gRPC.

In this example, we first create a node, add a name (Steven Spielberg) and age
attribute to it. We then create another node, add a name attribute (William Jones),
we then add a friend edge between the two nodes.
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	dgraphClient := graphp.NewDgraphClient(conn)
	req := client.Req{}

	nq := graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "name",
	}
	client.Str("Steven Spielberg", &nq)
	req.AddMutation(nq, client.SET)

	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "age",
	}
	if err = client.Int(25, &nq); err != nil {
		log.Fatal(err)
	}
	req.AddMutation(nq, client.SET)

	nq = graphp.NQuad{
		Subject:   "_:person2",
		Predicate: "name",
	}
	client.Str("William Jones", &nq)
	req.AddMutation(nq, client.SET)

	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "friend",
		ObjectId:  "_:person2",
	}
	req.AddMutation(nq, client.SET)

	resp, err := dgraphClient.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}


Dgraph would have assigned uids to these nodes.
See https://docs.dgraph.io/master/query-language/#assigning-uid for more details
on how assigning a new uid works.
We now query for these things.
	person1Uid := resp.AssignedUids["person1"]
	person2Uid := resp.AssignedUids["person2"]

	req = client.Req{}
	req.SetQuery(fmt.Sprintf(`
	{
		me(id: %v) {
			_uid_
			name
			age
			friend {
				_uid_
				name
			}
		}
	}`, client.Uid(person1Uid)))
	resp, err = dgraphClient.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}

	person1 := resp.N[0].Children[0]
	props := person1.Properties
	name := props[0].Value.GetStrVal()
	fmt.Println("Name: ", name)

	fmt.Println("Age: ", props[1].Value.GetIntVal())

	person2 := person1.Children[0]
	fmt.Printf("%v name: %v\n", person2.Attribute, person2.Properties[0].Value.GetStrVal())

This is how we delete the friend edge between the two nodes.
	nq = graphp.NQuad{
		Subject:   client.Uid(person1Uid),
		Predicate: "friend",
		ObjectId:  client.Uid(person2Uid),
	}
	req = client.Req{}
	req.AddMutation(nq, client.DEL)
	resp, err = dgraphClient.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}

Checkout the BatchMutation example if you want to upload large amounts of data quickly.

For more details checkout https://docs.dgraph.io/master/clients/#go.
*/
package client
