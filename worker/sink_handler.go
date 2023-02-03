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

package worker

import (
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shopify/sarama"
	"github.com/pkg/errors"
	"github.com/xdg/scram"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

type SinkMessage struct {
	Meta  SinkMeta
	Key   []byte
	Value []byte
}

type SinkMeta struct {
	Topic string
}

type Sink interface {
	// send in bulk to the sink
	Send(messages []SinkMessage) error
	// close sink
	Close() error
}

const (
	defaultSinkFileName = "sink.log"
)

func GetSink(conf *z.SuperFlag) (Sink, error) {
	switch {
	case conf.GetString("kafka") != "":
		return newKafkaSink(conf)
	case conf.GetPath("file") != "":
		return newFileSink(conf)
	}
	return nil, errors.New("sink config is not provided")
}

// Kafka client is not concurrency safe.
// Its the responsibility of callee to manage the concurrency.
type kafkaSinkClient struct {
	client   sarama.Client
	producer sarama.SyncProducer
}

func newKafkaSink(config *z.SuperFlag) (Sink, error) {
	if config.GetString("kafka") == "" {
		return nil, errors.New("brokers are not provided for the kafka config")
	}

	saramaConf := sarama.NewConfig()
	saramaConf.ClientID = "Dgraph"
	saramaConf.Producer.Partitioner = sarama.NewHashPartitioner
	saramaConf.Producer.Return.Successes = true
	saramaConf.Producer.Return.Errors = true

	if config.GetBool("tls") && config.GetPath("ca-cert") == "" {
		tlsCfg := x.TLSBaseConfig()
		var pool *x509.CertPool
		var err error
		if pool, err = x509.SystemCertPool(); err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool
		saramaConf.Net.TLS.Enable = true
		saramaConf.Net.TLS.Config = tlsCfg
	} else if config.GetPath("ca-cert") != "" {
		tlsCfg := x.TLSBaseConfig()
		var pool *x509.CertPool
		var err error
		if pool, err = x509.SystemCertPool(); err != nil {
			return nil, err
		}
		caFile, err := ioutil.ReadFile(config.GetPath("ca-cert"))
		if err != nil {
			return nil, errors.Wrap(err, "unable to read ca cert file")
		}
		if !pool.AppendCertsFromPEM(caFile) {
			return nil, errors.New("not able to append certificates")
		}
		tlsCfg.RootCAs = pool
		cert := config.GetPath("client-cert")
		key := config.GetPath("client-key")
		if cert != "" && key != "" {
			cert, err := tls.LoadX509KeyPair(cert, key)
			if err != nil {
				return nil, errors.Wrap(err, "unable to load client cert and key")
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
		}
		saramaConf.Net.TLS.Enable = true
		saramaConf.Net.TLS.Config = tlsCfg
	}

	if config.GetString("sasl-user") != "" && config.GetString("sasl-password") != "" {
		saramaConf.Net.SASL.Enable = true
		saramaConf.Net.SASL.User = config.GetString("sasl-user")
		saramaConf.Net.SASL.Password = config.GetString("sasl-password")
	}
	mechanism := config.GetString("sasl-mechanism")
	if mechanism != "" {
		switch mechanism {
		case sarama.SASLTypeSCRAMSHA256:
			saramaConf.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
			saramaConf.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &scramClient{HashGeneratorFcn: sha256.New}
			}
		case sarama.SASLTypeSCRAMSHA512:
			saramaConf.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
			saramaConf.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &scramClient{HashGeneratorFcn: sha512.New}
			}
		case sarama.SASLTypePlaintext:
			saramaConf.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		default:
			return nil, errors.Errorf("Invalid SASL mechanism. Valid mechanisms are: %s, %s and %s",
				sarama.SASLTypePlaintext, sarama.SASLTypeSCRAMSHA256, sarama.SASLTypeSCRAMSHA512)
		}
	}

	brokers := strings.Split(config.GetString("kafka"), ",")
	client, err := sarama.NewClient(brokers, saramaConf)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create kafka client")
	}
	producer, err := sarama.NewSyncProducerFromClient(client)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create producer from kafka client")
	}
	return &kafkaSinkClient{
		client:   client,
		producer: producer,
	}, nil
}

func (k *kafkaSinkClient) Send(messages []SinkMessage) error {
	if len(messages) == 0 {
		return nil
	}
	msgs := make([]*sarama.ProducerMessage, len(messages))
	for i, m := range messages {
		msgs[i] = &sarama.ProducerMessage{
			Topic: m.Meta.Topic,
			Key:   sarama.ByteEncoder(m.Key),
			Value: sarama.ByteEncoder(m.Value),
		}
	}
	return k.producer.SendMessages(msgs)
}

func (k *kafkaSinkClient) Close() error {
	_ = k.producer.Close()
	return k.client.Close()
}

// this is only for testing purposes. Ideally client wouldn't want file based sink
type fileSink struct {
	// log writer is buffered. Do take care of that while testing
	fileWriter *x.LogWriter
}

func (f *fileSink) Send(messages []SinkMessage) error {
	for _, m := range messages {
		_, err := f.fileWriter.Write([]byte(fmt.Sprintf("{ \"key\": \"%d\", \"value\": %s}\n",
			binary.BigEndian.Uint64(m.Key), string(m.Value))))
		if err != nil {
			return errors.Wrap(err, "unable to add message in the file sink")
		}
	}
	return nil
}

func (f *fileSink) Close() error {
	return f.fileWriter.Close()
}

func newFileSink(path *z.SuperFlag) (Sink, error) {
	dir := path.GetPath("file")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.Wrap(err, "unable to create directory for file sink")
	}

	fp, err := filepath.Abs(filepath.Join(dir, defaultSinkFileName))
	if err != nil {
		return nil, errors.Wrap(err, "unable to find file sink path")
	}

	w := &x.LogWriter{
		FilePath: fp,
		MaxSize:  100,
		MaxAge:   10,
	}
	if w, err = w.Init(); err != nil {
		return nil, errors.Wrap(err, "unable to init the file writer ")
	}
	return &fileSink{
		fileWriter: w,
	}, nil
}

type scramClient struct {
	*scram.Client
	*scram.ClientConversation
	scram.HashGeneratorFcn
}

func (sc *scramClient) Begin(userName, password, authzID string) (err error) {
	sc.Client, err = sc.HashGeneratorFcn.NewClient(userName, password, authzID)
	if err != nil {
		return err
	}
	sc.ClientConversation = sc.Client.NewConversation()
	return nil
}

func (sc *scramClient) Step(challenge string) (response string, err error) {
	response, err = sc.ClientConversation.Step(challenge)
	return
}

func (sc *scramClient) Done() bool {
	return sc.ClientConversation.Done()
}
