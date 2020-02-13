// +build !oss

/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package worker

import (
	"context"

	"github.com/dgraph-io/dgraph/ee/backup"
	"github.com/dgraph-io/dgraph/protos/pb"

	"github.com/pkg/errors"
)

func ProcessRestoreRequest(ctx context.Context, req *pb.Restore) error {
	memState := GetMembershipState()
	current_groups := make([]uint32, 0)
	for gid, _ := range memState.GetGroups() {
		current_groups = append(current_groups, gid)
	}

	creds := backup.Credentials{
		AccessKey:    req.AccessKey,
		SecretKey:    req.SecretKey,
		SessionToken: req.SessionToken,
		Anonymous:    req.Anonymous,
	}
	if err := backup.Verify(req.Location, req.BackupId, &creds, current_groups); err != nil {
		return errors.Wrapf(err, "failed to verify backup")
	}

	backup.FillRestoreCredentials(req.Location, &req)

	return nil
}

// Restore implements the Worker interface.
func (w *grpcWorker) Restore(ctx context.Context, req *pb.Restore) (*pb.Status, error) {
	return nil, nil
}
