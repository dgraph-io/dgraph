/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/dgraph-io/dgraph/graphql/schema"
)

// Reference: https://github.com/apollographql/subscriptions-transport-ws/blob/master/PROTOCOL.md
const (
	// Graphql subscription protocol name.
	protocolGraphQLWS = "graphql-ws"
	// Message type to initiate the connection.
	initMsg = "connection_init"
	// Message type to indicate the subscription message is acked by the server.
	ackMsg = "connection_ack"
	// Message type to start the subscription.
	startMsg = "start"
	// Message type of subscription response.
	dataMsg = "data"
	// Message type for terminating the subscription.
	terminateMsg = "connection_terminate"
	// Message type to indicate that given message is of error type
	errorMsg = "error"
)

type operationMessage struct {
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Type    string          `json:"type"`
}

// GraphQLSubscriptionClient uses apollo subscription protocol to subscribe on GraphQL server.
type GraphQLSubscriptionClient struct {
	conn *websocket.Conn
	id   string
}

// NewGraphQLSubscription returns graphql subscription client.
func NewGraphQLSubscription(url string, req *schema.Request, subscriptionPayload string) (*GraphQLSubscriptionClient, error) {
	header := http.Header{
		"Sec-WebSocket-Protocol": []string{protocolGraphQLWS},
	}

	dialer := websocket.DefaultDialer
	dialer.EnableCompression = true
	conn, _, err := dialer.Dial(url, header)
	if err != nil {
		return nil, err
	}
	// Initialize subscription.
	init := operationMessage{
		Type:    initMsg,
		Payload: []byte(subscriptionPayload),
	}

	// Send Intialization message to the graphql server.
	if err = conn.WriteJSON(init); err != nil {
		return nil, err
	}

	msg := operationMessage{}
	if err = conn.ReadJSON(&msg); err != nil {
		conn.Close()
		return nil, err
	}

	if msg.Type != ackMsg {
		fmt.Println(string(msg.Payload))
		return nil, fmt.Errorf("expected ack response from the server but got %+v", msg)
	}

	// We got ack, now send start the subscription by sending the query to the server.
	payload, err := json.Marshal(req)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Generate ID for the subscription.
	id := fmt.Sprintf("%d", rand.Int())
	msg.ID = id
	msg.Type = startMsg
	msg.Payload = payload

	if err = conn.WriteJSON(msg); err != nil {
		conn.Close()
		return nil, err
	}
	return &GraphQLSubscriptionClient{
		id:   id,
		conn: conn,
	}, nil
}

// RecvMsg recives graphql update from the server.
func (client *GraphQLSubscriptionClient) RecvMsg() ([]byte, error) {
	// Receive message from graphql server.
	msg := &operationMessage{}
	if err := client.conn.ReadJSON(msg); err != nil {
		return nil, err
	}

	// Check the message type.
	// TODO: handle complete, error... for testing. This should be enough.
	// We can do this, if we are planning to opensource this as subscription
	// library.
	if msg.Type == errorMsg {
		return nil, errors.New(string(msg.Payload))
	}
	if msg.Type != dataMsg {
		return nil, nil
	}
	return msg.Payload, nil
}

// Terminate will terminate the subscription.
func (client *GraphQLSubscriptionClient) Terminate() {
	msg := &operationMessage{
		ID:   client.id,
		Type: terminateMsg,
	}
	_ = client.conn.WriteJSON(msg)
	_ = client.conn.Close()
}
