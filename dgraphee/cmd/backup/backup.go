/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"bytes"
	"context"
	"hash/crc32"
	"io"
	"io/ioutil"
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	"google.golang.org/grpc"
)

type backup struct {
	target, source handler
	incremental    bool
	dst            string
	start          time.Time // To get time elapsel.
	zeroconn       *grpc.ClientConn
}

func (b *backup) process() error {
	tempFile, err := ioutil.TempFile(opt.tmpDir, "dgraph-backup-*")
	if err != nil {
		return err
	}
	defer tempFile.Close()

	zc := pb.NewZeroClient(b.zeroconn)
	stream, err := zc.Backup(context.Background(), &pb.BackupRequest{})
	if err != nil {
		return err
	}

	h := crc32.NewIEEE()
	for {
		res, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		csum := h.Sum(res.Data)
		if !bytes.Equal(csum, res.Checksum) {
			x.Printf("Warning: data checksum failed: %x != %x\n", csum, res.Checksum)
			continue
		}

		if _, err := pbutil.WriteDelimited(tempFile, res); err != nil {
			return x.Errorf("Backup: could not save to temp file: %s\n", err)
		}
	}

	if err := b.target.Copy(tempFile.Name(), b.dst); err != nil {
		return x.Errorf("Backup: could not copy to destination: %s\n", err)
	}

	return nil
}
