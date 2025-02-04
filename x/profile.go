/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"fmt"
	"os"
	"runtime"

	"github.com/pkg/profile"
	"github.com/spf13/viper"
)

// Stopper is an interface tasked with stopping the profiling process.
type Stopper interface {
	Stop()
}

// StartProfile starts a new mode for profiling.
func StartProfile(conf *viper.Viper) Stopper {
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
