package kafka

import (
	"fmt"
	"strings"
	"time"

	"github.com/Shopify/sarama"
	"github.com/dgraph-io/badger"
	bpb "github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

type Callback func(proposal *pb.Proposal) error
type state struct {
	partition int32 // the same partition number that is used for producing and consuming messages
}
type Cancel func()

var pstore *badger.DB
var cb Callback
var s state
var producer sarama.SyncProducer

const (
	dgraphTopic = "dgraph"
	dgraphGroup = "dgraph-consumer-group"
)

func Init(db *badger.DB) {
	pstore = db
}

func consumeMsg(pom sarama.PartitionOffsetManager, message *sarama.ConsumerMessage) error {
	kafkaMsg := &pb.KafkaMsg{}
	if err := kafkaMsg.Unmarshal(message.Value); err != nil {
		return fmt.Errorf("error while unmarshaling from consumed message: %v", err)
	}
	proposal := &pb.Proposal{}
	if kafkaMsg.KvList != nil {
		//consumeList(kafkaMsg.KvList)
		proposal.Kv = kafkaMsg.KvList.Kv
	}
	if kafkaMsg.State != nil {
		proposal.State = kafkaMsg.State
	}
	if kafkaMsg.Schema != nil {
		proposal.Mutations = &pb.Mutations{
			Schema: []*pb.SchemaUpdate{kafkaMsg.Schema},
		}
	}
	if err := cb(proposal); err != nil {
		return fmt.Errorf("error while calling callback for proposal: %+v", proposal)
	}
	// Marking of the message must be done after the message has been securely processed.
	// Otherwise marking a message prematurely may result in message loss
	// if the server crashes right after the message is marked.
	pom.MarkOffset(message.Offset+1, "")
	return nil
}

// setupKafkaSource will create a kafka consumer and and use it to receive updates
func SetupKafkaSource(c Callback, partition int32) {
	cb = c
	s.partition = partition

	sourceBrokers := Config.SourceBrokers
	glog.Infof("source kafka brokers: %v", sourceBrokers)
	if len(sourceBrokers) > 0 {
		pom, cancelPom, err := getPOM(sourceBrokers)
		if err != nil {
			glog.Errorf("error while getting the partition offset manager: %v", err)
			return
		}

		client, err := getKafkaConsumer(sourceBrokers)
		if err != nil {
			glog.Errorf("unable to get kafka consumer and will not receive updates: %v", err)
			return
		}

		var partConsumer sarama.PartitionConsumer

		nextOffset, _ := pom.NextOffset()
		partConsumer, err = client.ConsumePartition(dgraphTopic, s.partition, nextOffset)
		if err != nil {
			glog.Errorf("error while consuming from kafka: %v", err)
			return
		}

		go func() {
			for msg := range partConsumer.Messages() {
				if err := consumeMsg(pom, msg); err != nil {
					glog.Errorf("error while handling kafka msg: %v", err)
				}
			}

			glog.V(1).Infof("closing the kafka offset manager")
			cancelPom()
		}()

		glog.Info("kafka consumer up and running")
	}
}

// getKafkaConsumer tries to create a consumer by connecting to Kafka at the specified brokers.
// If an error errors while creating the consumer, this function will wait and retry up to 10 times
// before giving up and returning an error
func getKafkaConsumer(sourceBrokers string) (sarama.Consumer, error) {
	config := sarama.NewConfig()
	config.Version = sarama.V2_2_0_0

	addrs := strings.Split(sourceBrokers, ",")
	var consumer sarama.Consumer
	var err error
	for i := 0; i < 10; i++ {
		consumer, err = sarama.NewConsumer(addrs, config)
		if err == nil {
			break
		} else {
			glog.Errorf("unable to create the kafka consumer, "+
				"will retry in 5 seconds: %v", err)
			time.Sleep(5 * time.Second)
		}
	}

	return consumer, err
}

func getPOM(sourceBrokers string) (sarama.PartitionOffsetManager, Cancel, error) {
	addrs := strings.Split(sourceBrokers, ",")
	client, err := sarama.NewClient(addrs, nil)
	if err != nil {
		return nil, nil, err
	}

	om, err := sarama.NewOffsetManagerFromClient(dgraphGroup, client)
	if err != nil {
		return nil, nil, err
	}

	pom, err := om.ManagePartition(dgraphTopic, s.partition)
	return pom, func() {
		client.Close()
	}, nil
}

func PublishSchema(s *pb.SchemaUpdate) {
	if producer == nil {
		return
	}

	msg := &pb.KafkaMsg{
		Schema: s,
	}
	if err := produceMsg(msg); err != nil {
		glog.Errorf("error while publishing schema update to kafka: %v", err)
		return
	}

	glog.V(1).Infof("published schema update to kafka")
}

func PublishMembershipState(state *pb.MembershipState) {
	if producer == nil {
		return
	}

	msg := &pb.KafkaMsg{
		State: state,
	}
	if err := produceMsg(msg); err != nil {
		glog.Errorf("error while publishing membership state to kafka: %v", err)
		return
	}
	glog.V(2).Infof("published membership state to kafka: %+v", state)
}

// setupKafkaTarget will create a kafka producer and use it to send updates to
// the kafka cluster. The partition argument specifies which kafka partition will
// be used for the current raft group.
func SetupKafkaTarget(partition int32) {
	targetBrokers := Config.TargetBrokers
	glog.Infof("target kafka brokers: %v", targetBrokers)
	if len(targetBrokers) > 0 {
		s.partition = partition

		var err error
		producer, err = getKafkaProducer(targetBrokers)
		if err != nil {
			glog.Errorf("unable to create the kafka sync producer, and will not publish updates")
			return
		}

		cb := func(list *bpb.KVList) {
			kafkaMsg := &pb.KafkaMsg{
				KvList: list,
			}
			if err := produceMsg(kafkaMsg); err != nil {
				glog.Errorf("error while producing to Kafka: %v", err)
				return
			}

			glog.V(1).Infof("produced a list with %d messages to kafka", len(list.Kv))
		}

		go func() {
			// The Subscribe will go into an infinite loop,
			// hence we need to run it inside a separate go routine
			if err := pstore.Subscribe(context.Background(), cb, nil); err != nil {
				glog.Errorf("error while subscribing to the pstore: %v", err)
			}
		}()

		glog.V(1).Infof("subscribed to the pstore for updates")
	}
}

func produceMsg(msg *pb.KafkaMsg) error {
	msgBytes, err := msg.Marshal()
	if err != nil {
		return fmt.Errorf("unable to marshal the kv list: %v", err)
	}
	_, _, err = producer.SendMessage(&sarama.ProducerMessage{
		Topic:     dgraphTopic,
		Partition: s.partition,
		Value:     sarama.ByteEncoder(msgBytes),
	})
	return err
}

// getKafkaProducer tries to create a producer by connecting to Kafka at the specified brokers.
// If an error errors while creating the producer, this function will wait and retry up to 10 times
// before giving up and returning an error
func getKafkaProducer(targetBrokers string) (sarama.SyncProducer, error) {
	conf := sarama.NewConfig()
	conf.Producer.Return.Successes = true
	var producer sarama.SyncProducer
	var err error
	for i := 0; i < 10; i++ {
		producer, err = sarama.NewSyncProducer(strings.Split(targetBrokers, ","), conf)
		if err == nil {
			break
		} else {
			glog.Errorf("unable to create the kafka sync producer, "+
				"will retry in 5 seconds: %v", err)
			time.Sleep(5 * time.Second)
		}
	}
	return producer, err
}
