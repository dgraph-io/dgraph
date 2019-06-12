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
