package p2p

import (
	"testing"
)

var testServiceConfig = &ServiceConfig{
	BootstrapNode: "/ip4/104.131.131.82/tcp/4001/ipfs/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
	Port:          7001,
}

func TestStart(t *testing.T) {
	s, err := NewService(testServiceConfig)
	if err != nil {
		t.Fatalf("NewService error: %s", err)
	}

	err = s.Start()
	if err != nil {
		t.Errorf("Start error :%s", err)
	}
}

func TestStartDHT(t *testing.T) {
	s, err := NewService(testServiceConfig)
	if err != nil {
		t.Fatalf("NewService error: %s", err)
	}

	err = s.startDHT()
	if err != nil {
		t.Errorf("TestStartDHT error: %s", err)
	}
}

func TestBuildOpts(t *testing.T) {
	_, err := testServiceConfig.buildOpts()
	if err != nil {
		t.Fatalf("TestBuildOpts error: %s", err)
	}
}
