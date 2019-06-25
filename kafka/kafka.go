package kafka

import (
	"fmt"
	"strings"
	"time"

	"github.com/dgraph-io/badger/y"

	"github.com/Shopify/sarama"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/golang/glog"
)

type Callback func(proposal *pb.Proposal) error
type state struct {
	partition int32 // the same partition number that is used for producing and consuming messages
}
type Cancel func()

var cb Callback
var consumerCloser *y.Closer
var s state
var producer sarama.SyncProducer

const (
	dgraphTopic = "dgraph"
	dgraphGroup = "dgraph-consumer-group"
)

func consumeMsg(pom sarama.PartitionOffsetManager, message *sarama.ConsumerMessage) error {
	proposal := &pb.Proposal{}
	if err := proposal.Unmarshal(message.Value); err != nil {
		return fmt.Errorf("error while unmarshaling from consumed message: %v", err)
	}
	if err := cb(proposal); err != nil {
		return err
	}
	// Marking of the message must be done after the message has been securely processed.
	// Otherwise marking a message prematurely may result in message loss
	// if the server crashes right after the message is marked.
	pom.MarkOffset(message.Offset+1, "")
	return nil
}

func waitForPartitionCount(brokers []string, partition int32) error {
	config := sarama.NewConfig()
	config.Version = sarama.V2_2_0_0
	var admin sarama.ClusterAdmin
	var err error
	retries := 20
	for i := 0; i < retries; i++ {
		admin, err = sarama.NewClusterAdmin(brokers, config)
		if err == nil {
			break
		}
		glog.Warningf("error while creating cluster admin, will retry in 1s: %v", err)
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("error while creating cluster admin: %v", err)
	}

	expectedPartitionCount := int(partition + 1)

	for i := 0; i < retries; i++ {
		topics, err := admin.DescribeTopics([]string{dgraphTopic})
		if err != nil {
			return fmt.Errorf("error while describing topic: %v", err)
		}
		if len(topics) < 1 {
			return fmt.Errorf("topic metadata not found")
		}

		if len(topics[0].Partitions) >= expectedPartitionCount {
			return nil
		}

		glog.Warningf("current partition count %d, waiting for it to reach %d",
			len(topics[0].Partitions), expectedPartitionCount)
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("unable to meet the expected partition count %d",
		expectedPartitionCount)
}

// SetupKafkaSource will create a kafka consumer and and use it to receive updates
func SetupKafkaSource(c Callback, partition int32) {
	cb = c
	s.partition = partition

	sourceBrokers := Config.SourceBrokers
	glog.Infof("source kafka brokers: %v", sourceBrokers)
	if len(sourceBrokers) > 0 {
		brokers := strings.Split(sourceBrokers, ",")

		internalSetup := func() {
			if err := waitForPartitionCount(brokers, partition); err != nil {
				glog.Errorf("error while waiting for partition count: %v", err)
				return
			}

			pom, cancelPom, err := getPOM(brokers)
			if err != nil {
				glog.Errorf("error while getting the partition offset manager: %v", err)
				return
			}
			defer cancelPom()

			client, err := getKafkaConsumer(brokers)
			if err != nil {
				glog.Errorf("unable to get kafka consumer and will not receive updates: %v", err)
				return
			}

			nextOffset, _ := pom.NextOffset()
			glog.V(1).Infof("setting next offset to %d", nextOffset)

			var partConsumer sarama.PartitionConsumer
			partConsumer, err = client.ConsumePartition(dgraphTopic, s.partition, nextOffset)
			if err != nil {
				glog.Errorf("error while consuming from partition %s-%d: %v",
					dgraphTopic, s.partition, err)
				return
			}
			glog.Info("kafka consumer up and running")

			consumerCloser = y.NewCloser(1)
			for {
				select {
				case <-consumerCloser.HasBeenClosed():
					consumerCloser.Done()
					consumerCloser = nil
					return
				case msg := <-partConsumer.Messages():
					if err := consumeMsg(pom, msg); err != nil {
						glog.Errorf("error while handling kafka msg: %v", err)
					}
				}
			}
		}

		// since setting up the kafka producer may involve blocking and waiting
		// we run the logic inside a separate go routine to avoid blocking the raft main loop
		// for too long
		go internalSetup()
	}
}

// CancelKafkaSource will send a signal to the consumer loop and terminate message consuming from
// Kafka
func CancleKafkaSource() {
	if consumerCloser != nil {
		consumerCloser.SignalAndWait()
		glog.Infof("cancelled message consuming from kafka")
	}
}

// getKafkaConsumer tries to create a consumer by connecting to Kafka at the specified brokers.
// If an error errors while creating the consumer, this function will wait and retry up to 10 times
// before giving up and returning an error
func getKafkaConsumer(brokers []string) (sarama.Consumer, error) {
	config := sarama.NewConfig()
	config.Version = sarama.V2_2_0_0

	var consumer sarama.Consumer
	var err error
	retries := 10
	for i := 0; i < retries; i++ {
		consumer, err = sarama.NewConsumer(brokers, config)
		if err == nil {
			return consumer, nil
		} else {
			glog.Errorf("unable to create the kafka consumer, "+
				"will retry in 5 seconds: %v", err)
			time.Sleep(5 * time.Second)
		}
	}

	return nil, fmt.Errorf("unable to get consumer after retries")
}

func getPOM(brokers []string) (sarama.PartitionOffsetManager, Cancel, error) {
	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetOldest

	client, err := sarama.NewClient(brokers, config)
	if err != nil {
		return nil, nil, fmt.Errorf("error while creating client: %v", err)
	}

	om, err := sarama.NewOffsetManagerFromClient(dgraphGroup, client)
	if err != nil {
		return nil, nil,
			fmt.Errorf("error while creating offset manager from client: %v", err)
	}

	pom, err := om.ManagePartition(dgraphTopic, s.partition)
	return pom, func() {
		client.Close()
	}, nil
}

// SetupKafkaTarget will create a kafka producer and use it to send updates to
// the kafka cluster. The partition argument specifies which kafka partition will
// be used for the current raft group.
func SetupKafkaTarget(partition int32) {
	targetBrokers := Config.TargetBrokers
	s.partition = partition
	glog.Infof("target kafka brokers: %v", targetBrokers)
	if len(targetBrokers) > 0 {
		brokers := strings.Split(targetBrokers, ",")

		internalSetup := func() {
			if err := waitForPartitionCount(brokers, partition); err != nil {
				glog.Errorf("error while waiting for partition count: %v", err)
				return
			}

			var err error
			producer, err = getKafkaProducer(brokers)
			if err != nil {
				glog.Errorf("unable to create the kafka sync producer, and will not publish updates")
				return
			}

			glog.V(1).Infof("created kafka producer")
		}

		// since setting up the kafka producer may involve blocking and waiting
		// we run the logic inside a separate go routine to avoid blocking the raft main loop
		// for too long
		go internalSetup()
	}
}

// CancelKafkaTarget will invalidate the producer, and hence disable sending more messages to Kafka
func CancelKafkaTarget() {
	producer = nil
	glog.Infof("cancelled message producing to kafka")
}

func PublishMsg(proposal *pb.Proposal) {
	if producer == nil {
		return
	}

	msgBytes, err := proposal.Marshal()
	if err != nil {
		glog.Errorf("unable to marshal the kv list: %v", err)
		return
	}
	_, _, err = producer.SendMessage(&sarama.ProducerMessage{
		Topic:     dgraphTopic,
		Partition: s.partition,
		Value:     sarama.ByteEncoder(msgBytes),
	})
	if err != nil {
		glog.Errorf("error while publishing msg to kafka: %v", err)
		return
	}

	glog.V(1).Infof("published proposal to kafka partition %d: %+v", s.partition, proposal)
}

// getKafkaProducer tries to create a producer by connecting to Kafka at the specified brokers.
// If an error errors while creating the producer, this function will wait and retry up to 10 times
// before giving up and returning an error
func getKafkaProducer(brokers []string) (sarama.SyncProducer, error) {
	conf := sarama.NewConfig()
	conf.Producer.Return.Successes = true
	conf.Producer.Partitioner = sarama.NewManualPartitioner

	var producer sarama.SyncProducer
	var err error
	retries := 10
	for i := 0; i < retries; i++ {
		producer, err = sarama.NewSyncProducer(brokers, conf)
		if err == nil {
			return producer, nil
		} else {
			glog.Errorf("unable to create the kafka sync producer, "+
				"will retry in 5 seconds: %v", err)
			time.Sleep(5 * time.Second)
		}
	}
	return nil, fmt.Errorf("unable to get producer after retries")
}
