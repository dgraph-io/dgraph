package p2p

import (
	"os"
	"path"
	"reflect"
	"testing"
)

// test setupKey method
func TestSetupKey(t *testing.T) {
	testDir := path.Join(os.TempDir(), "gossamer-test")

	defer os.RemoveAll(testDir)

	configA := &Config{
		DataDir: testDir,
	}

	err := configA.setupKey()
	if err != nil {
		t.Fatal(err)
	}

	configB := &Config{
		DataDir: testDir,
	}

	err = configB.setupKey()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(configA.privateKey, configB.privateKey) {
		t.Error("Private keys should match")
	}

	configC := &Config{
		RandSeed: 1,
	}

	err = configC.setupKey()
	if err != nil {
		t.Fatal(err)
	}

	configD := &Config{
		RandSeed: 2,
	}

	err = configD.setupKey()
	if err != nil {
		t.Fatal(err)
	}

	if reflect.DeepEqual(configC.privateKey, configD.privateKey) {
		t.Error("Private keys should not match")
	}
}

// test build configuration method
func TestBuildConfig(t *testing.T) {
	testDir := path.Join(os.TempDir(), "gossamer-test")

	defer os.RemoveAll(testDir)

	cfg := &Config{
		DataDir: testDir,
	}

	err := cfg.build()
	if err != nil {
		t.Fatal(err)
	}

	testKey, err := generateKey(0, testDir)
	if err != nil {
		t.Fatal(err)
	}

	testCfg := &Config{
		BootstrapNodes: nil,
		ProtocolId:     "",
		Port:           0,
		RandSeed:       0,
		NoBootstrap:    false,
		NoMdns:         false,
		DataDir:        testDir,
		privateKey:     testKey,
	}

	if reflect.DeepEqual(cfg, testCfg) {
		t.Error("Configurations should the same")
	}
}
