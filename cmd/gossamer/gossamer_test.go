package main_test

import (
	"encoding/json"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	cfg "github.com/ChainSafe/gossamer/config"
	"github.com/ChainSafe/gossamer/p2p"
	"github.com/ChainSafe/gossamer/polkadb"
	log "github.com/Chainsafe/log15"
	"github.com/rendon/testcli"
)

var binaryname = "gossamer-test"

const timeFormat = "2006-01-02T15:04:05-0700"

func setup() *os.File {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = build.Default.GOPATH
	}
	r := exec.Command("go", "build", "-o", gopath+"/bin/gossamer-test")
	err := r.Run()
	if err != nil {
		log.Crit("could not execute binary", "executable", binaryname, "err", err)
		os.Exit(1)
	}
	run := exec.Command(`gossamer-test`)
	err = run.Run()
	if err != nil {
		log.Crit("could not execute binary", "executable", binaryname, "err", err)
		os.Exit(1)
	}

	tmpFile, err := ioutil.TempFile(os.TempDir(), "prefix-")
	if err != nil {
		log.Crit("Cannot create temporary file", err)
		os.Exit(1)
	}

	testConfig := fmt.Sprintf("%s%s%s%v%s%s%v%s%s%s",
		"[ServiceConfig]\n", "BootstrapNodes = []\n", "Port = ",
		7005, "\n", "RandSeed = ", 0, "\n\n", "[DbConfig]\n",
		"Datadir = \"\"\x0A")

	_, err = tmpFile.Write([]byte(testConfig))
	if err != nil {
		log.Crit("could not write to test config", "config", "config-test.toml", "err", err)
		os.Exit(1)
	}
	return tmpFile
}

func teardown(tempFile *os.File) {
	err := os.Chdir("../gossamer")
	if err != nil {
		log.Crit("could not change dir", "err", err)
		os.Exit(1)
	}
	if err := os.RemoveAll("./chaindata"); err != nil {
		log.Warn("removal of temp directory bin failed", "err", err)
	}
	if err := os.Remove(tempFile.Name()); err != nil {
		log.Warn("cannot create temp file", err)
	}
}

func TestInitialOutput(t *testing.T) {
	tempFile := setup()
	defer teardown(tempFile)

	testcli.Run("gossamer-test")
	if !testcli.Success() {
		t.Fatalf("Expected to succeed, but failed: %s", testcli.Error())
	}
	output := fmt.Sprintf("%s%v%s", "t=", time.Now().Format(timeFormat), " lvl=info msg=\"🕸️ starting p2p service\" blockchain=gossamer\x0A")
	if !testcli.StdoutContains(output) {
		t.Fatalf("Expected %q to contain %q", testcli.Stdout(), output)
	}
	if !reflect.DeepEqual(testcli.Stdout(), output) {
		t.Fatalf("actual = %s, expected = %s", testcli.Stdout(), output)
	}
}

func TestCliArgs(t *testing.T) {
	tempFile := setup()
	res := expectedResponses()
	defer teardown(tempFile)
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"dumpconfig with config set", []string{"--config", tempFile.Name(), res[0]}, res[0]},
		{"config specified", []string{"--config", tempFile.Name()}, res[1]},
		{"default config", []string{}, res[1]},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			gopath := os.Getenv("GOPATH")
			if gopath == "" {
				gopath = build.Default.GOPATH
			}
			dir := gopath + "/bin/"
			cmd := exec.Command(path.Join(dir, binaryname), tt.args...)
			actual, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.ContainsAny(string(actual), tt.expected) {
				t.Fatalf("actual = %s, expected = %s", string(actual), tt.expected)
			}
		})
	}
}

func expectedResponses() []string {
	var testConfig = &cfg.Config{
		ServiceConfig: &p2p.ServiceConfig{
			BootstrapNodes: []string{},
			Port:           7001,
			RandSeed:       32,
		},
		DbConfig: polkadb.DbConfig{
			Datadir: "",
		},
	}

	b, err := json.Marshal(testConfig)
	if err != nil {
		log.Error("could not marshal testConfig for expected response", "err", err)
	}
	dumpCfgExp := fmt.Sprintf("%v", string(b))
	startingChainMsg := fmt.Sprintf("%s%v%s", "t=", time.Now().Format(timeFormat),
		" lvl=info msg=\"🕸️ starting p2p service\" blockchain=gossamer\x0A")

	response := []string{dumpCfgExp, startingChainMsg}

	return response
}
