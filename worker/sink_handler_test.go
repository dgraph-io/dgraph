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

package worker

import (
	"context"
	"testing"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/require"
)

func TestKafkaSink(t *testing.T) {
	_, err := kafka.DialLeader(context.Background(), "tcp", "localhost:49212", "testingtopic", 0)
	require.Nil(t, err)
	client, err := NewKafkaSinkClient(&KafkaConfig{
		addr:  "localhost:49212",
		topic: "testingtopic",
	})
	require.Nil(t, err)

	err = client.SendMessage([]byte("key1"), []byte("value1"))
	require.Nil(t, err)
	verifyViaConsumer(t)
}

func verifyViaConsumer(t *testing.T) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{"localhost:49212"},
		Topic:   "testingtopic",
		GroupID: "dgraph_consumer",
	})
	message, err := r.FetchMessage(context.Background())
	require.Nil(t, err)
	require.Equal(t, message.Key, []byte("key1"))
	require.Equal(t, message.Value, []byte("value1"))
}
