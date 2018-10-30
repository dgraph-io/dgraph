/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/dgraph-io/dgraph/protos/pb"
)

const dgraphBackupTempPrefix = "dgraph-backup-*"

type writer struct {
	file string
	dst  handler
	tmp  *os.File
}

func (w *Worker) newWriter() (*writer, error) {
	// dst is where we will copy the data
	dst, err := findHandler("/tmp/dgraph/")
	if err != nil {
		return nil, err
	}
	// tmp file is our main working file
	tmp, err := ioutil.TempFile("", dgraphBackupTempPrefix)
	if err != nil {
		return nil, err
	}
	// file: 1283719371922.12.3242423938.dgraph-backup
	file := fmt.Sprintf("%s.%d.%d%s",
		w.SeqTs, w.GroupId, w.ReadTs, dgraphBackupSuffix)
	return &writer{
		dst:  dst,
		tmp:  tmp,
		file: file,
	}, nil
}

func (w *writer) Send(kvs *pb.KVS) error {
	var err error
	for _, kv := range kvs.Kv {
		err = binary.Write(w.tmp, binary.LittleEndian, kv.Val)
		if err != nil {
			return err
		}
	}
	return nil
}
