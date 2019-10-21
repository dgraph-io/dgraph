package p2p

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestSetupPrivKey(t *testing.T) {
	tmpDir, err := ioutil.TempDir(os.TempDir(), "gossamer-test")
	if err != nil {
		t.Fatal(err)
	}
	configA := &Config{
		BootstrapNodes: nil,
		Port:           0,
		RandSeed:       0,
		NoBootstrap:    true,
		NoMdns:         true,
		DataDir:        tmpDir,
		privateKey:     nil,
	}

	err = configA.setupPrivKey()
	if err != nil {
		t.Fatal(err)
	}

	// Load private key
	configB := &(*configA)
	configB.privateKey = nil

	err = configB.setupPrivKey()
	if err != nil {
		t.Fatal(err)
	}

	if !configA.privateKey.Equals(configB.privateKey) {
		t.Errorf("keys don't match. publicA: %s publicB: %s", configA.privateKey.GetPublic(), configB.privateKey.GetPublic())
	}
}
