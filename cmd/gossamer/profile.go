// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"os"
	"runtime"
	"runtime/pprof"

	"github.com/urfave/cli"
)

func beginProfile(ctx *cli.Context) (func(), error) {
	cpuStopFunc, err := cpuProfile(ctx)
	if err != nil {
		return nil, err
	}

	memStopFunc, err := memProfile(ctx)
	if err != nil {
		return nil, err
	}

	return func() {
		if cpuStopFunc != nil {
			cpuStopFunc()
		}

		if memStopFunc != nil {
			memStopFunc()
		}
	}, nil
}

func cpuProfile(ctx *cli.Context) (func(), error) {
	cpuProfFile := ctx.GlobalString(CPUProfFlag.Name)
	if cpuProfFile == "" {
		return nil, nil
	}

	cpuFile, err := os.Create(cpuProfFile)
	if err != nil {
		return nil, err
	}

	if err := pprof.StartCPUProfile(cpuFile); err != nil {
		return nil, err
	}

	return func() {
		pprof.StopCPUProfile()
		if err := cpuFile.Close(); err != nil {
			logger.Error("failed to close file", "file", cpuFile.Name())
		}
	}, nil
}

func memProfile(ctx *cli.Context) (func(), error) {
	memProfFile := ctx.GlobalString(MemProfFlag.Name)
	if memProfFile == "" {
		return nil, nil
	}

	memFile, err := os.Create(memProfFile)
	if err != nil {
		return nil, err
	}

	return func() {
		runtime.GC()
		if err := pprof.WriteHeapProfile(memFile); err != nil {
			logger.Error("could not write memory profile", "error", err)
		}

		if err := memFile.Close(); err != nil {
			logger.Error("failed to close file", "file", memFile.Name())
		}
	}, nil
}
