/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"flag"
	"os"
	"strings"

	"github.com/dgraph-io/dgraph/dgraph/cmd/migrate"

	"github.com/dgraph-io/dgraph/dgraph/cmd/alpha"
	"github.com/dgraph-io/dgraph/dgraph/cmd/bulk"
	"github.com/dgraph-io/dgraph/dgraph/cmd/cert"
	"github.com/dgraph-io/dgraph/dgraph/cmd/conv"
	"github.com/dgraph-io/dgraph/dgraph/cmd/counter"
	"github.com/dgraph-io/dgraph/dgraph/cmd/debug"
	"github.com/dgraph-io/dgraph/dgraph/cmd/live"
	"github.com/dgraph-io/dgraph/dgraph/cmd/version"
	"github.com/dgraph-io/dgraph/dgraph/cmd/zero"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "dgraph",
	Short: "Dgraph: Distributed Graph Database",
	Long: `
Dgraph is a horizontally scalable and distributed graph database,
providing ACID transactions, consistent replication and linearizable reads.
It's built from ground up to perform for a rich set of queries. Being a native
graph database, it tightly controls how the data is arranged on disk to optimize
for query performance and throughput, reducing disk seeks and network calls in a
cluster.
` + x.BuildDetails(),
	PersistentPreRunE: cobra.NoArgs,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	initCmds()

	// Convinces glog that Parse() has been called to avoid noisy logs.
	// https://github.com/kubernetes/kubernetes/issues/17162#issuecomment-225596212
	x.Check(flag.CommandLine.Parse([]string{}))

	// Dumping the usage in case of an error makes the error messages harder to see.
	RootCmd.SilenceUsage = true

	x.CheckfNoLog(RootCmd.Execute())
}

var rootConf = viper.New()

// subcommands initially contains all default sub-commands.
var subcommands = []*x.SubCommand{
	&bulk.Bulk, &cert.Cert, &conv.Conv, &live.Live, &alpha.Alpha, &zero.Zero, &version.Version,
	&debug.Debug, &counter.Increment, &migrate.Migrate,
}

func initCmds() {
	RootCmd.PersistentFlags().String("cwd", "",
		"Change working directory to the path specified. The parent must exist.")
	RootCmd.PersistentFlags().String("profile_mode", "",
		"Enable profiling mode, one of [cpu, mem, mutex, block]")
	RootCmd.PersistentFlags().Int("block_rate", 0,
		"Block profiling rate. Must be used along with block profile_mode")
	RootCmd.PersistentFlags().String("config", "",
		"Configuration file. Takes precedence over default values, but is "+
			"overridden to values set with environment variables and flags.")
	RootCmd.PersistentFlags().Bool("bindall", true,
		"Use 0.0.0.0 instead of localhost to bind to all addresses on local machine.")
	RootCmd.PersistentFlags().Bool("expose_trace", false,
		"Allow trace endpoint to be accessible from remote")
	_ = rootConf.BindPFlags(RootCmd.PersistentFlags())

	// Add all existing global flag (eg: from glog) to rootCmd's flags
	RootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	// Always set stderrthreshold=0. Don't let users set it themselves.
	x.Check(RootCmd.PersistentFlags().Set("stderrthreshold", "0"))
	x.Check(RootCmd.PersistentFlags().MarkDeprecated("stderrthreshold",
		"Dgraph always sets this flag to 0. It can't be overwritten."))

	for _, sc := range subcommands {
		RootCmd.AddCommand(sc.Cmd)
		sc.Conf = viper.New()
		sc.Conf.BindPFlags(sc.Cmd.Flags())
		sc.Conf.BindPFlags(RootCmd.PersistentFlags())
		sc.Conf.AutomaticEnv()
		sc.Conf.SetEnvPrefix(sc.EnvPrefix)
		// Options that contain a "." should use "_" in its place when provided as an
		// environment variable.
		sc.Conf.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	}
	// For bash shell completion
	RootCmd.AddCommand(shellCompletionCmd())

	cobra.OnInitialize(func() {
		// When run inside docker, the working_dir is created by root even if afterward
		// the process is run as another user. Creating a new working directory here
		// ensures that it is owned by the user instead, so it can be cleaned up without
		// requiring root when using a bind volume (i.e. a mounted host directory).
		if cwd := rootConf.GetString("cwd"); cwd != "" {
			err := os.Mkdir(cwd, 0777)
			if err != nil && !os.IsExist(err) {
				x.Fatalf("unable to create directory: %v", err)
			}
			x.CheckfNoTrace(os.Chdir(cwd))
		}

		cfg := rootConf.GetString("config")
		if cfg == "" {
			return
		}
		for _, sc := range subcommands {
			sc.Conf.SetConfigFile(cfg)
			x.Check(errors.Wrapf(sc.Conf.ReadInConfig(), "reading config"))
			setGlogFlags(sc.Conf)
		}
	})
}

// setGlogFlags function sets the glog flags based on the configuration.
// We need to manually set the flags from configuration because glog reads
// values from flags, not configuration.
func setGlogFlags(conf *viper.Viper) {
	// List of flags taken from
	// https://github.com/golang/glog/blob/master/glog.go#L399
	// and https://github.com/golang/glog/blob/master/glog_file.go#L41
	glogFlags := [...]string{
		"log_dir", "logtostderr", "alsologtostderr", "v",
		"stderrthreshold", "vmodule", "log_backtrace_at",
	}
	for _, gflag := range glogFlags {
		// Set value of flag to the value in config
		if stringValue, ok := conf.Get(gflag).(string); ok {
			// Special handling for log_backtrace_at flag because the flag is of
			// type tracelocation. The nil value for tracelocation type is
			// ":0"(See https://github.com/golang/glog/blob/master/glog.go#L322).
			// But we can't set nil value for the flag because of
			// https://github.com/golang/glog/blob/master/glog.go#L374
			// Skip setting value if log_backstrace_at is nil in config.
			if gflag == "log_backtrace_at" && stringValue == ":0" {
				continue
			}
			x.Check(flag.Lookup(gflag).Value.Set(stringValue))
		}
	}
}

func shellCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Generates shell completion scripts for bash or zsh",
	}

	// bash subcommand
	cmd.AddCommand(&cobra.Command{
		Use:   "bash",
		Short: "bash shell completion",
		Long: `To load bash completion run:
dgraph completion bash > dgraph-completion.sh

To configure your bash shell to load completions for each session,
add to your bashrc:

# ~/.bashrc or ~/.profile
. path/to/dgraph-completion.sh
`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return RootCmd.GenBashCompletion(os.Stdout)
		},
	})

	// zsh subcommand
	cmd.AddCommand(&cobra.Command{
		Use:   "zsh",
		Short: "zsh shell completion",
		Long: `To generate zsh completion run:
dgraph completion zsh > _dgraph

Then install the completion file somewhere in your $fpath or
$_compdir paths. You must enable the compinit and compinstall plugins.

For more information, see the official Zsh docs:
http://zsh.sourceforge.net/Doc/Release/Completion-System.html
`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return RootCmd.GenZshCompletion(os.Stdout)
		},
	})

	return cmd
}
