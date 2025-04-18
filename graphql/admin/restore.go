/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/hypermodeinc/dgraph/v25/edgraph"
	"github.com/hypermodeinc/dgraph/v25/graphql/resolve"
	"github.com/hypermodeinc/dgraph/v25/graphql/schema"
	"github.com/hypermodeinc/dgraph/v25/protos/pb"
	"github.com/hypermodeinc/dgraph/v25/worker"
)

type restoreInput struct {
	Location          string
	BackupId          string
	BackupNum         int
	IncrementalFrom   int
	IsPartial         bool
	EncryptionKeyFile string
	AccessKey         string
	SecretKey         pb.Sensitive
	SessionToken      pb.Sensitive
	Anonymous         bool
	VaultAddr         string
	VaultRoleIDFile   string
	VaultSecretIDFile string
	VaultPath         string
	VaultField        string
	VaultFormat       string
}

type restoreTenantInput struct {
	RestoreInput  restoreInput
	FromNamespace uint64
}

func resolveTenantRestore(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	input, err := getRestoreTenantInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}
	glog.Infof("Got restore request: %+v", input)

	req := pb.RestoreRequest{
		Location:                input.RestoreInput.Location,
		BackupId:                input.RestoreInput.BackupId,
		BackupNum:               uint64(input.RestoreInput.BackupNum),
		IncrementalFrom:         uint64(input.RestoreInput.IncrementalFrom),
		IsPartial:               input.RestoreInput.IsPartial,
		EncryptionKeyFile:       input.RestoreInput.EncryptionKeyFile,
		AccessKey:               input.RestoreInput.AccessKey,
		SecretKey:               input.RestoreInput.SecretKey,
		SessionToken:            input.RestoreInput.SessionToken,
		Anonymous:               input.RestoreInput.Anonymous,
		VaultAddr:               input.RestoreInput.VaultAddr,
		VaultRoleidFile:         input.RestoreInput.VaultRoleIDFile,
		VaultSecretidFile:       input.RestoreInput.VaultSecretIDFile,
		VaultPath:               input.RestoreInput.VaultPath,
		VaultField:              input.RestoreInput.VaultField,
		VaultFormat:             input.RestoreInput.VaultFormat,
		FromNamespace:           input.FromNamespace,
		IsNamespaceAwareRestore: true,
	}
	return restore(ctx, m, req)
}

func resolveRestore(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	input, err := getRestoreInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}
	glog.Infof("Got restore request, location: %v, backupId: %v, backupNum: %v, incrementalFrom: %d, isPartial: %v",
		input.Location, input.BackupId, input.BackupNum, input.IncrementalFrom, input.IsPartial)

	req := pb.RestoreRequest{
		Location:                input.Location,
		BackupId:                input.BackupId,
		BackupNum:               uint64(input.BackupNum),
		IncrementalFrom:         uint64(input.IncrementalFrom),
		IsPartial:               input.IsPartial,
		EncryptionKeyFile:       input.EncryptionKeyFile,
		AccessKey:               input.AccessKey,
		SecretKey:               input.SecretKey,
		SessionToken:            input.SessionToken,
		Anonymous:               input.Anonymous,
		VaultAddr:               input.VaultAddr,
		VaultRoleidFile:         input.VaultRoleIDFile,
		VaultSecretidFile:       input.VaultSecretIDFile,
		VaultPath:               input.VaultPath,
		VaultField:              input.VaultField,
		VaultFormat:             input.VaultFormat,
		IsNamespaceAwareRestore: false,
	}

	return restore(ctx, m, req)
}

func restore(ctx context.Context, m schema.Mutation, req pb.RestoreRequest) (*resolve.Resolved, bool) {
	wg := &sync.WaitGroup{}
	if err := worker.ProcessRestoreRequest(context.Background(), &req, wg); err != nil {
		glog.Warningf("error processing restore request: %+v, err: %v", req, err)
		return resolve.DataResult(
			m,
			map[string]interface{}{m.Name(): map[string]interface{}{
				"code": "Failure",
			}},
			schema.GQLWrapLocationf(err, m.Location(), "resolving %s failed", m.Name()),
		), false
	}

	go func() {
		wg.Wait()
		edgraph.InitializeAcl(nil)
	}()

	return resolve.DataResult(
		m,
		map[string]interface{}{m.Name(): map[string]interface{}{
			"code":    "Success",
			"message": "Restore operation started.",
		}},
		nil,
	), true
}

func getRestoreInput(m schema.Mutation) (*restoreInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input restoreInput
	if err := json.Unmarshal(inputByts, &input); err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}
	if err := verifyRestoreInput(input); err != nil {
		return nil, err
	}

	return &input, nil
}

func getRestoreTenantInput(m schema.Mutation) (*restoreTenantInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input restoreTenantInput
	if err := json.Unmarshal(inputByts, &input); err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}
	if err := verifyRestoreInput(input.RestoreInput); err != nil {
		return nil, err
	}

	return &input, nil
}

func verifyRestoreInput(input restoreInput) error {
	if input.BackupNum < 0 {
		err := errors.Errorf("backupNum value should be equal or greater than zero")
		return schema.GQLWrapf(err, "couldn't get input argument")
	}
	return nil
}
