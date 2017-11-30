package x

import (
	"fmt"
	"os"
	"runtime"

	"github.com/pkg/profile"
	"github.com/spf13/viper"
)

func StartProfile(conf *viper.Viper) interface {
	Stop()
} {
	profileMode := conf.GetString("profile_mode")
	switch profileMode {
	case "cpu":
		return profile.Start(profile.CPUProfile)
	case "mem":
		return profile.Start(profile.MemProfile)
	case "mutex":
		return profile.Start(profile.MutexProfile)
	case "block":
		blockRate := conf.GetInt("block_rate")
		runtime.SetBlockProfileRate(blockRate)
		return profile.Start(profile.BlockProfile)
	case "":
		// do nothing
		return noOpStopper{}
	default:
		fmt.Printf("Invalid profile mode: %q\n", profileMode)
		os.Exit(1)
		return noOpStopper{}
	}
}

type noOpStopper struct{}

func (noOpStopper) Stop() {}
