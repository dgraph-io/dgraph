package kafka

type KafkaOptions struct {
	TargetBrokers string
	SourceBrokers string
}

var Config KafkaOptions
