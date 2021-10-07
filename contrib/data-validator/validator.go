package main

import (
	"flag"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/klog"
)

var (
	Validator Command
	opts      Options
)

type Command struct {
	Cmd  *cobra.Command
	Conf *viper.Viper
}

type Options struct {
	file   string
	format string
}

func init() {
	Validator.Cmd = &cobra.Command{
		Use:   "validator",
		Short: "A tool to do side-by-side comparison of dgraph clusters",
		RunE:  run,
	}

	flags := Validator.Cmd.Flags()
	flags.StringVar(&opts.file,
		"file", "", "Path of the data file")
	flags.StringVar(&opts.format,
		"format", "json", "Format of the data file")
	Validator.Conf = viper.New()
	Validator.Conf.BindPFlags(flags)

	fs := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(fs)
	Validator.Cmd.Flags().AddGoFlagSet(fs)
}

func main() {
	flag.CommandLine.Set("logtostderr", "true")
	check(flag.CommandLine.Parse([]string{}))
	check(Validator.Cmd.Execute())
}

func run(cmd *cobra.Command, args []string) error {
	return nil
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
