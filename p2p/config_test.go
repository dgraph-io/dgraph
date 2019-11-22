package p2p

import (
	"os"
	"path"
	"reflect"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	testDir := path.Join(os.TempDir(), "gossamer-test")

	defer os.RemoveAll(testDir)

	keyA, err := generateKey(0, testDir)
	if err != nil {
		t.Fatal(err)
	}

	keyB, err := generateKey(0, testDir)
	if err != nil {
		t.Fatal(err)
	}

	if reflect.DeepEqual(keyA, keyB) {
		t.Error("Generated keys should not match")
	}

	keyC, err := generateKey(1, testDir)
	if err != nil {
		t.Fatal(err)
	}

	keyD, err := generateKey(1, testDir)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(keyC, keyD) {
		t.Error("Generated keys should match")
	}
}

func TestSetupPrivateKey(t *testing.T) {
	testDir := path.Join(os.TempDir(), "gossamer-test")

	defer os.RemoveAll(testDir)

	configA := &Config{
		DataDir: testDir,
	}

	err := configA.setupPrivKey()
	if err != nil {
		t.Fatal(err)
	}

	configB := &Config{
		DataDir: testDir,
	}

	err = configB.setupPrivKey()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(configA.privateKey, configB.privateKey) {
		t.Error("Private keys should match")
	}

	configC := &Config{
		RandSeed: 1,
	}

	err = configC.setupPrivKey()
	if err != nil {
		t.Fatal(err)
	}

	configD := &Config{
		RandSeed: 2,
	}

	err = configD.setupPrivKey()
	if err != nil {
		t.Fatal(err)
	}

	if reflect.DeepEqual(configC.privateKey, configD.privateKey) {
		t.Error("Private keys should not match")
	}
}

func TestBuildOptions(t *testing.T) {
	testDir := path.Join(os.TempDir(), "gossamer-test")

	defer os.RemoveAll(testDir)

	configA := &Config{
		DataDir: testDir,
	}

	_, err := configA.buildOpts()
	if err != nil {
		t.Fatal(err)
	}

	key, err := generateKey(0, testDir)
	if err != nil {
		t.Fatal(err)
	}

	configB := &Config{
		BootstrapNodes: nil,
		ProtocolId:     "",
		Port:           0,
		RandSeed:       0,
		NoBootstrap:    false,
		NoMdns:         false,
		DataDir:        testDir,
		privateKey:     key,
	}

	if reflect.DeepEqual(configA, configB) {
		t.Error("Configurations should the same")
	}
}
