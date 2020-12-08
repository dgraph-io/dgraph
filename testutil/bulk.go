package testutil

import (
	"context"
	"fmt"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"
)

type LiveOpts struct {
	Alpha      string
	Zero       string
	RdfFile    string
	SchemaFile string
	Dir        string
}

func LiveLoad(opts LiveOpts) error {
	liveCmd := exec.Command(DgraphBinaryPath(), "live",
		"--files", opts.RdfFile,
		"--schema", opts.SchemaFile,
		"--alpha", opts.Alpha,
		"--zero", opts.Zero,
	)

	if opts.Dir != "" {
		liveCmd.Dir = opts.Dir
	}

	if out, err := liveCmd.Output(); err != nil {
		fmt.Printf("Error %v\n", err)
		fmt.Printf("Output %v\n", string(out))
		return err
	}

	return nil
}

type BulkOpts struct {
	Zero          string
	Shards        int
	RdfFile       string
	SchemaFile    string
	GQLSchemaFile string
	Dir           string
}

func BulkLoad(opts BulkOpts) error {
	bulkCmd := exec.Command(DgraphBinaryPath(), "bulk",
		"-f", opts.RdfFile,
		"-s", opts.SchemaFile,
		"-g", opts.GQLSchemaFile,
		"--http", "localhost:"+strconv.Itoa(FreePort(0)),
		"--reduce-shards="+strconv.Itoa(opts.Shards),
		"--map-shards="+strconv.Itoa(opts.Shards),
		"--store-xids=true",
		"--zero", opts.Zero,
	)

	if opts.Dir != "" {
		bulkCmd.Dir = opts.Dir
	}
	if out, err := bulkCmd.CombinedOutput(); err != nil {
		fmt.Printf("Error %v\n", err)
		fmt.Printf("Output %v\n", string(out))
		return err
	}

	return nil
}

func MakeDirEmpty(dir []string) error {
	for _, d := range dir {
		_ = os.RemoveAll(d)
		err := os.MkdirAll(d, 0755)
		if err != nil {
			return err
		}
	}
	return nil
}

func FreePort(port int) int {
	// Linux reuses ports in FIFO order. So a port that we listen on and then
	// release will be free for a long time.
	for {
		// p + 5080 and p + 9080 must lie within [20000, 60000]
		offset := 15000 + rand.Intn(30000)
		p := port + offset
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			listener.Close()
			return offset
		}
	}
}

func StartAlphas(compose string) error {
	cmd := exec.Command("docker-compose", "-f", compose, "-p", DockerPrefix,
		"up", "-d", "--force-recreate")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("Error while bringing up alpha node. Prefix: %s. Error: %v\n", DockerPrefix, err)
		fmt.Printf("Output %v\n", string(out))
		return err
	}
	var err error
	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)
		var resp *http.Response
		resp, err = http.Get("http://" + ContainerAddr("alpha1", 8080) + "/health")
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if err == nil && resp.StatusCode == http.StatusOK {
			return nil
		}
	}

	return err
}

func StopAlphas(compose string) {
	dg, err := DgraphClient(ContainerAddr("alpha1", 9080))
	if err == nil {
		_ = dg.Alter(context.Background(), &api.Operation{
			DropAll: true,
		})
	}

	cmd := exec.CommandContext(context.Background(), "docker-compose", "-f", compose,
		"-p", DockerPrefix, "rm", "-f", "-s", "-v")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error while bringing down cluster. Prefix: %s. Error: %v\n", DockerPrefix, err)
	}
}
