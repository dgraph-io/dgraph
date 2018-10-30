/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/golang/glog"
	"github.com/matttproud/golang_protobuf_extensions/pbutil"
)

const dgraphBackupTempPrefix = "dgraph-backup-*"
const dgraphBackupSuffix = ".dgraph-backup"

type writer struct {
	file string
	dst  handler
	tmp  *os.File
}

func (w *writer) save() error {
	glog.Infof("Saving backup to: %q", w.file)
	if err := w.dst.Copy(w.tmp.Name(), w.file); err != nil {
		return err
	}
	glog.V(3).Infof("copied %q to %q on target ...", w.tmp.Name(), w.file)
	// we are done done, cleanup.
	return w.cleanup()
}

func (w *writer) cleanup() error {
	// always remove the temp file
	defer func() {
		if err := os.Remove(w.tmp.Name()); err != nil {
			// let the user know there's baggage left behind. they might have to delete by hand.
			glog.Errorf("failed to remove temp file %q: %s", w.tmp.Name(), err)
		}
	}()

	glog.V(3).Info("cleaning up ...")
	if err := w.tmp.Close(); err != nil {
		return err
	}
	return nil
}

func newWriter(worker *Worker) (*writer, error) {
	var w writer
	var err error

	// dst is the final destination for data.
	w.dst, err = getSchemeHandler(worker.TargetURI)
	if err != nil {
		return nil, err
	}

	// tmp file is our main working file.
	// we will prepare this file and then copy to dst when done.
	w.tmp, err = ioutil.TempFile("", dgraphBackupTempPrefix)
	if err != nil {
		glog.Errorf("could not create temp file: %s\n", err)
		return nil, err
	}
	glog.V(3).Infof("temp file: %q", w.tmp.Name())

	// file name: 1283719371922.12.3242423938.dgraph-backup
	w.file = fmt.Sprintf("%s.%d.%d%s",
		worker.SeqTs, worker.GroupId, worker.ReadTs, dgraphBackupSuffix)
	glog.V(3).Infof("target file %q", w.file)

	return &w, err
}

// Send implements the stream.kvStream interface.
// It writes the received KV into the temp file as a delimited binary chain.
// Returns error if the writing fails, nil on success.
func (w *writer) Send(kvs *pb.KVS) error {
	var err error
	for _, kv := range kvs.Kv {
		_, err = pbutil.WriteDelimited(w.tmp, kv)
		if err != nil {
			return err
		}
	}
	return nil
}
