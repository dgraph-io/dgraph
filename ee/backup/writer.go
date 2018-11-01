/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"encoding/binary"
	"fmt"
	"net/url"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/golang/glog"
)

const dgraphBackupTempPrefix = "dgraph-backup-*"
const dgraphBackupSuffix = ".dgraph-backup"

// writer handles the writes from stream.Orchestrate. It implements the kvStream interface.
type writer struct {
	h    handler
	file string
}

// newWriter parses the requested target URI, finds a handler and then tries to create a session.
// Target URI format:
//   [scheme]://[host]/[path]?[args]
//
// Target URI parts:
//   scheme - service handler, one of: "s3", "gs", "az", "http", "file"
//     host - remote address. ex: "dgraph.s3.amazonaws.com"
//     path - directory, bucket or container at target. ex: "/dgraph/backups/"
//     args - specific arguments that are ok to appear in logs.
//
// Examples:
//   s3://dgraph.s3.amazonaws.com/dgraph/backups?useSSL=1
//   gs://dgraph/backups/
//   as://dgraph-container/backups/
//   http://backups.dgraph.io/upload
//   file:///tmp/dgraph/backups or /tmp/dgraph/backups
func newWriter(r *Request) (*writer, error) {
	u, err := url.Parse(r.TargetURI)
	if err != nil {
		return nil, err
	}
	// find handler for this URI scheme
	h, err := getHandler(u.Scheme)
	if err != nil {
		return nil, err
	}
	f := fmt.Sprintf("%s-g%d-r%d%s", r.UnixTs, r.GroupId, r.ReadTs, dgraphBackupSuffix)
	glog.V(3).Infof("target file: %q", f)

	// create session at
	sess := &session{host: u.Host, path: u.Path, file: f, args: u.Query()}
	if err := h.Open(sess); err != nil {
		return nil, err
	}

	return &writer{h: h, file: f}, nil
}

func (w *writer) close() error {
	return w.h.Close()
}

// write uses the data length as delimiter.
// XXX: we could use CRC for restore.
func (w *writer) write(kv *pb.KV) error {
	if err := binary.Write(w.h, binary.LittleEndian, uint64(kv.Size())); err != nil {
		return err
	}
	b, err := kv.Marshal()
	if err != nil {
		return err
	}
	_, err = w.h.Write(b)
	return err
}

// Send implements the stream.kvStream interface.
// It writes the received KV the target as a delimited binary chain.
// Returns error if the writing fails, nil on success.
func (w *writer) Send(kvs *pb.KVS) error {
	var err error
	for _, kv := range kvs.Kv {
		err = w.write(kv)
		if err != nil {
			return err
		}
	}
	return nil
}
