package dgraphtest

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

type DgraphPerf struct {
	Name string
	cc   ClusterConfig
	mc   MetricConfig
	rc   ResourceConfig
	dc   DatasetConfig
}

func NewRemoteCluster(conf ClusterConfig) (*LocalCluster, error) {
	c := &LocalCluster{conf: conf}
	if err := c.init(); err != nil {
		c.Cleanup()
		return nil, err
	}

	return c, nil
}

var defaultClusterConfig ClusterConfig

func RunPerfTest(b *testing.B, name string, perfTest func(cluster Cluster, b *testing.B)) {
	fmt.Println("Running: ", name)

	var cluster Cluster
	config, found := os.LookupEnv("Config")
	if found {
		// Setup cluster based on Resource config in ssh machines
		// using config
		var dgraphPerf DgraphPerf
		err := json.Unmarshal([]byte(config), &dgraphPerf)

		if err != nil {
			return
		}

		cluster, _ = NewRemoteCluster(dgraphPerf.cc)

	} else {
		// Setup cluster locally based on default config
		cluster, _ = NewLocalCluster(defaultClusterConfig)

	}
	perfTest(cluster, b)

}
