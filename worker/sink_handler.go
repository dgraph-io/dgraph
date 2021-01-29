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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
)

type SinkHandler interface {
	// send message to the sink
	SendMessage(key []byte, message []byte) error
	// close sink
	Close() error
}

type KafkaConfig struct {
	addr  string
	topic string

	// tls config
	tlsEnabled bool
	caCert     []byte
	clientCert []byte
	clientKey  []byte

	// sasl config
	saslEnabled  bool
	saslUser     string
	saslPassword string
}

// Kafka client is not concurrency safe.
// Its the responsibility of callee to manage the concurrency.
type kafkaSinkClient struct {
	writer *kafka.Writer
}

func NewKafkaSinkClient(config *KafkaConfig) (SinkHandler, error) {
	if config == nil {
		return nil, nil
	}
	tp := &kafka.Transport{}
	if config.tlsEnabled {
		tlsCfg := &tls.Config{}
		var pool *x509.CertPool
		var err error
		if pool, err = x509.SystemCertPool(); err != nil {
			return nil, err
		}
		if !pool.AppendCertsFromPEM(config.caCert) {
			return nil, errors.New("not able to append certificates")
		}
		tlsCfg.RootCAs = pool
		cert := config.clientCert
		key := config.clientKey
		if cert != nil && key != nil {
			cert, err := tls.LoadX509KeyPair(string(cert), string(key))
			if err != nil {
				return nil, err
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
		}
		tp.TLS = tlsCfg
	}

	if config.saslEnabled {
		tp.SASL = &plain.Mechanism{
			Username: config.saslUser,
			Password: config.saslPassword,
		}
	}
	tp.ClientID = "Dgraph"
	w := &kafka.Writer{
		Addr:         kafka.TCP(config.addr),
		Topic:        config.topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll,
		Transport:    tp,
		Completion: func(msg []kafka.Message, err error) {
			if err == nil {
				return
			}
			glog.Error("error writing to kafka", err)
		},
	}
	return &kafkaSinkClient{
		writer: w,
	}, nil
}

// send message send it async.
func (k *kafkaSinkClient) SendMessage(key []byte, message []byte) error {
	return k.writer.WriteMessages(context.Background(), kafka.Message{
		Key:   key,
		Value: message,
	})
}

func (k *kafkaSinkClient) Close() error {
	return k.writer.Close()
}

type fileSink struct {
	fileWriter *x.LogWriter
}

var iter uint64

func (f *fileSink) SendMessage(key []byte, message []byte) error {
	if atomic.LoadUint64(&iter) < 1000 && atomic.LoadUint64(&iter) > 100 {
		atomic.AddUint64(&iter, 10)
		return errors.New("")
	}
	atomic.AddUint64(&iter, 1)
	_, err := f.fileWriter.Write([]byte(fmt.Sprintf("{ \"key\": %s, \"value\": %s}\n",
		string(key), string(message))))
	return err
}

func (f *fileSink) Close() error {
	return f.fileWriter.Close()
}

func NewFileBasedSink(path string) (SinkHandler, error) {
	var err error
	w := &x.LogWriter{
		FilePath:      path,
		MaxSize:       100,
		MaxAge:        10,
		EncryptionKey: nil,
		Compress:      false,
	}
	if w, err = w.Init(); err != nil {
		return nil, err
	}
	return &fileSink{
		fileWriter: w,
	}, nil
}
