package sample

import (
	"context"
	"fmt"
	"os"

	"github.com/hypermodeinc/dgraph/v25/dgraph/cmd/dgraphimport"
	"github.com/hypermodeinc/dgraph/v25/x"
	"github.com/spf13/cobra"
)

var (
	Import x.SubCommand
)

func init() {
	Import.Cmd = &cobra.Command{
		Use:   "import123",
		Short: "Run the import tool",
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
		Annotations: map[string]string{"group": "tool"},
	}
	Import.Cmd.SetHelpTemplate(x.NonRootTemplate)
}

func run() {
	if err := importP(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func importP() error {
	url := "dgraph://0.0.0.0:9080"
	return dgraphimport.Import(context.Background(), url, "/home/shiva/workspace/dgraph-work/benchmarks/data/out")
}
