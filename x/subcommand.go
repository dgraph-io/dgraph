package x

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type SubCommand struct {
	Cmd  *cobra.Command
	Conf *viper.Viper

	EnvPrefix string
}
