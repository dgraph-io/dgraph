package dgraphtest

import (
	"bytes"
	"io/ioutil"
	"log"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func NewRemoteCluster(conf ClusterConfig) (*LocalCluster, error) {
	c := &LocalCluster{conf: conf}
	if err := c.init(); err != nil {
		c.Cleanup()
		return nil, err
	}

	return c, nil
}

var defaultClusterConfig ClusterConfig

func RunCmdInResource(cmd string, resources ResourceDetails) error {
	// ssh into the client and set up the client
	pemBytes, err := ioutil.ReadFile(resources.Keys())
	if err != nil {
		log.Panic(err)
	}
	signer, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		log.Panicf("parse key failed:%v", err)
	}
	config := &ssh.ClientConfig{
		User:            "ubuntu",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	log.Printf("Dialing %s\n", resources.IP())

	conn, err := ssh.Dial("tcp", resources.IP()+":22", config)
	if err != nil {
		log.Panicf("Failed to dial: %s\n", err)
	}
	defer conn.Close()
	log.Printf("Successfully connected to %s\n", conn.RemoteAddr())

	session, err := conn.NewSession()
	if err != nil {
		log.Panicf("session failed:%v", err)
	}
	defer session.Close()
	log.Println("Running command ", cmd)
	var stdoutBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	err = session.Run(cmd)

	if err != nil {
		log.Panicf("Run failed:%v", err)
	}

	log.Printf(">%s", strings.TrimSpace(stdoutBuf.String()))
	return nil
}

func RunPerfTest(b *testing.B, conf ClusterConfig, perfTest func(cluster Cluster, b *testing.B)) {
	cluster, err := NewLocalCluster(conf)
	require.NoError(b, err)
	defer cluster.Cleanup()
	cluster.Start()
	perfTest(cluster, b)
}
