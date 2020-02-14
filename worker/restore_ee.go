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

// ProcessRestoreRequest verifies the backup data and sends a restore proposal to each group.
func ProcessRestoreRequest(ctx context.Context, req *pb.Restore) error {
	if req == nil {
		return errors.Errorf("Restore request cannot be nil")
	}

	UpdateMembershipState(ctx)
	memState := GetMembershipState()

	currentGroups := make([]uint32, 0)
	for gid := range memState.GetGroups() {
		currentGroups = append(currentGroups, gid)
	}

	creds := backup.Credentials{
		AccessKey:    req.AccessKey,
		SecretKey:    req.SecretKey,
		SessionToken: req.SessionToken,
		Anonymous:    req.Anonymous,
	}
	if err := backup.Verify(req.Location, req.BackupId, &creds, currentGroups); err != nil {
		return errors.Wrapf(err, "failed to verify backup")
	}

	backup.FillRestoreCredentials(req.Location, req)
	req.RestoreTs = State.GetTimestamp(false)

	for _, gid := range currentGroups {

	}

	return nil
}

// Restore implements the Worker interface.
func (w *grpcWorker) Restore(ctx context.Context, req *pb.Restore) (*pb.Status, error) {
	return nil, nil
}
