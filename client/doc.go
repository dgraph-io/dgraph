/*
This package provides helper function for interacting with the Dgraph server.
You can use it to run mutations and queries. You can also use BatchMutation
to upload data concurrently. It communicates with the server using gRPC.

Simple example to run some mutations and queries.
	// Connecting to Dgraph server.
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())

	// Creating a new client.
	dgraphClient := graphp.NewDgraphClient(conn)
	req := client.Req{}

	// Lets add some mutations.
	// _:person1 tells Dgraph to assign a new Uid and is the preferred way of creating new nodes.
	// See https://docs.dgraph.io/master/query-language/#assigning-uid for more details.
	// Adding the name attribute to the person.
	nq := graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "name",
	}
	client.Str("Steven Spielberg", &nq)
	req.AddMutation(nq, client.SET)

	// Adding another attribute, age.
	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "age",
	}
	if err = client.Int(25, &nq); err != nil {
		log.Fatal(err)
	}
	req.AddMutation(nq, client.SET)

	// Lets create another person and add a name for it.
	nq = graphp.NQuad{
		Subject:   "_:person2",
		Predicate: "name",
	}
	client.Str("William Jones", &nq)
	req.AddMutation(nq, client.SET)

	// Lets connect the two nodes together.
	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "friend",
		ObjectId:  "_:person2",
	}
	req.AddMutation(nq, client.SET)

	// Now lets run the request with all these mutations.
	resp, err := dgraphClient.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}

	// Getting the assigned uids.
	person1Uid := resp.AssignedUids["person1"]
	person2Uid := resp.AssignedUids["person2"]

	// Lets initiate a new request and query for the data.
	req = client.Req{}
	// Lets set the starting node id to person1Uid, using the helper function client.Uid().
	req.SetQuery(fmt.Sprintf("{ me(id: %v) { _uid_ name now birthday loc salary age married friend {_uid_ name} } }", client.Uid(person1Uid)),
		map[string]string{})
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

	// Deleting an edge.
	nq = graphp.NQuad{
		Subject:   client.Uid(person1Uid),
		Predicate: "friend",
		ObjectId:  client.Uid(person2Uid),
	}
	req = client.Req{}
	req.AddMutation(nq, client.DEL)
	// Lets run the request with all these mutations.
	resp, err = dgraphClient.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}

Checkout the BatchMutation example if you wan't to upload large amounts of data quickly.

For more details checkout https://docs.dgraph.io/master/clients/#go.
*/
package client
