// +build !oss

/*
 * Copyright 2021 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package alpha

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
)

func setupLambdaServer(closer *z.Closer) {
	// If lambda-url is set, then don't launch the lambda servers from dgraph.
	if len(x.Config.GraphQL.GetString("lambda-url")) > 0 {
		return
	}

	num := int(x.Config.GraphQL.GetUint32("lambda-cnt"))
	port := int(x.Config.GraphQL.GetUint32("lambda-port"))
	if num == 0 {
		return
	}

	glog.Infoln("Setting up lambda servers")
	dgraphUrl := fmt.Sprintf("http://localhost:%d", httpPort())
	// Entry point of the script is index.js.
	filename := filepath.Join(x.WorkerConfig.TmpDir, "index.js")

	dir := "dist"
	files, err := jsLambda.ReadDir(dir)
	x.Check(err)
	for _, file := range files {
		// The separator for embedded files is forward-slash even on Windows.
		data, err := jsLambda.ReadFile(dir + "/" + file.Name())
		x.Check(err)
		filename := filepath.Join(x.WorkerConfig.TmpDir, file.Name())
		file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		x.Check(err)
		_, err = file.Write(data)
		x.Check(err)
		x.Check(file.Close())
	}

	for i := 0; i < num; i++ {
		go func(i int) {
			for {
				select {
				case <-closer.HasBeenClosed():
					break
				default:
					cmd := exec.CommandContext(closer.Ctx(), "node", filename)
					cmd.Env = append(cmd.Env, fmt.Sprintf("PORT=%d", port+i))
					cmd.Env = append(cmd.Env, fmt.Sprintf("DGRAPH_URL="+dgraphUrl))
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					glog.Infof("Running node command: %+v\n", cmd)
					err := cmd.Run()
					if err != nil {
						glog.Errorf("Lambda server idx: %d stopped with error %v", i, err)
					}
					time.Sleep(2 * time.Second)
				}
			}
		}(i)
	}
}
