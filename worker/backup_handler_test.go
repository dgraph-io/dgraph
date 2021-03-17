// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterManifestDefault(t *testing.T) {
	manifests := []*Manifest{
		{
			Type:      "full",
			BackupId:  "aa",
			BackupNum: 1,
		},
		{
			Type:      "full",
			BackupId:  "ab",
			BackupNum: 1,
		},
	}
	expected := []*Manifest{
		{
			Type:      "full",
			BackupId:  "ab",
			BackupNum: 1,
		},
	}
	manifests, err := filterManifests(manifests, "")
	require.NoError(t, err)
	require.Equal(t, manifests, expected)
}

func TestFilterManifestSelectSeries(t *testing.T) {
	manifests := []*Manifest{
		{
			Type:      "full",
			BackupId:  "aa",
			BackupNum: 1,
		},
		{
			Type:      "full",
			BackupId:  "ab",
			BackupNum: 1,
		},
	}
	expected := []*Manifest{
		{
			Type:      "full",
			BackupId:  "aa",
			BackupNum: 1,
		},
	}
	manifests, err := filterManifests(manifests, "aa")
	require.NoError(t, err)
	require.Equal(t, manifests, expected)
}

func TestFilterManifestMissingBackup(t *testing.T) {
	manifests := []*Manifest{
		{
			Type:      "full",
			BackupId:  "aa",
			BackupNum: 1,
		},
		{
			Type:      "incremental",
			BackupId:  "aa",
			BackupNum: 3,
		},
	}
	_, err := filterManifests(manifests, "aa")
	require.Error(t, err)
	require.Contains(t, err.Error(), "found a manifest with backup number")
}

func TestFilterManifestMissingFirstBackup(t *testing.T) {
	manifests := []*Manifest{
		{
			Type:      "incremental",
			BackupId:  "aa",
			BackupNum: 2,
		},
		{
			Type:      "incremental",
			BackupId:  "aa",
			BackupNum: 3,
		},
	}
	_, err := filterManifests(manifests, "aa")
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected a BackupNum value of 1 for first manifest")
}

func TestFilterManifestDifferentSeries(t *testing.T) {
	manifests := []*Manifest{
		{
			Type:      "full",
			BackupId:  "aa",
			BackupNum: 1,
		},
		{
			Type:      "incremental",
			BackupId:  "ab",
			BackupNum: 2,
		},
	}
	_, err := filterManifests(manifests, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "found a manifest with backup ID")
}
