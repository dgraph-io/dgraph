/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

package resolve

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dgraph-io/dgraph/graphql/authorization"

	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

type webhookPayload struct {
	Resolver   string             `json:"resolver"`
	AccessJWT  string             `json:"X-Dgraph-AccessToken,omitempty"`
	AuthHeader *authHeaderPayload `json:"authHeader,omitempty"`
	Event      eventPayload       `json:"event"`
}

type authHeaderPayload struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type eventPayload struct {
	Typename  string              `json:"__typename"`
	Operation schema.MutationType `json:"operation"`
	Add       *addEvent           `json:"add,omitempty"`
	Update    *updateEvent        `json:"update,omitempty"`
	Delete    *deleteEvent        `json:"delete,omitempty"`
}

type addEvent struct {
	RootUIDs []string      `json:"rootUIDs"`
	Input    []interface{} `json:"input"`
}

type updateEvent struct {
	RootUIDs    []string    `json:"rootUIDs"`
	SetPatch    interface{} `json:"setPatch"`
	RemovePatch interface{} `json:"removePatch"`
}

type deleteEvent struct {
	RootUIDs []string `json:"rootUIDs"`
}

// sendWebhookEvent forms an HTTP payload required for the webhooks configured with @lambdaOnMutate
// directive, and then sends that payload to the lambda URL configured with Alpha. There is no
// guarantee that the payload will be delivered successfully to the lambda server.
func sendWebhookEvent(ctx context.Context, m schema.Mutation, rootUIDs []string) {
	accessJWT, _ := x.ExtractJwt(ctx)
	var authHeader *authHeaderPayload
	if m.GetAuthMeta() != nil {
		authHeader = &authHeaderPayload{
			Key:   m.GetAuthMeta().GetHeader(),
			Value: authorization.GetJwtToken(ctx),
		}
	}

	payload := webhookPayload{
		Resolver:   "$webhook",
		AccessJWT:  accessJWT,
		AuthHeader: authHeader,
		Event: eventPayload{
			Typename:  m.MutatedType().Name(),
			Operation: m.MutationType(),
		},
	}

	switch payload.Event.Operation {
	case schema.AddMutation:
		input, _ := m.ArgValue(schema.InputArgName).([]interface{})
		payload.Event.Add = &addEvent{
			RootUIDs: rootUIDs,
			Input:    input,
		}
	case schema.UpdateMutation:
		inp, _ := m.ArgValue(schema.InputArgName).(map[string]interface{})
		payload.Event.Update = &updateEvent{
			RootUIDs:    rootUIDs,
			SetPatch:    inp["set"],
			RemovePatch: inp["remove"],
		}
	case schema.DeleteMutation:
		payload.Event.Delete = &deleteEvent{RootUIDs: rootUIDs}
	}

	b, err := json.Marshal(payload)
	if err != nil {
		// don't care to send the payload if there are JSON marshalling errors
		return
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	// don't care for the response/errors, if any.
	_, _ = schema.MakeHttpRequest(nil, http.MethodPost, x.Config.GraphQL.GetString("lambda-url"),
		headers, b)
}
