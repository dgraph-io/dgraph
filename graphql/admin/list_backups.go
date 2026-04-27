/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/v25/graphql/resolve"
	"github.com/dgraph-io/dgraph/v25/graphql/schema"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/worker"
	"github.com/dgraph-io/dgraph/v25/x"
)

type lsBackupInput struct {
	Location     string
	AccessKey    string
	SecretKey    pb.Sensitive
	SessionToken pb.Sensitive
	Anonymous    bool
	ForceFull    bool
	SinceDate    string `json:"sinceDate"`
	UntilDate    string `json:"untilDate"`
	LastNDays    int    `json:"lastNDays"`
	FullManifest bool   `json:"fullManifest"`
}

type group struct {
	GroupId    uint32   `json:"groupId,omitempty"`
	Predicates []string `json:"predicates,omitempty"`
}

type manifest struct {
	Type      string   `json:"type,omitempty"`
	Since     uint64   `json:"since,omitempty"`
	ReadTs    uint64   `json:"read_ts,omitempty"`
	Groups    []*group `json:"groups,omitempty"`
	BackupId  string   `json:"backupId,omitempty"`
	BackupNum uint64   `json:"backupNum,omitempty"`
	Path      string   `json:"path,omitempty"`
	Encrypted bool     `json:"encrypted,omitempty"`
}

// parseGraphQLDate parses a YYYY-MM-DD or RFC3339 date string.
func parseGraphQLDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.Errorf("date string must not be empty")
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	return t.UTC(), err
}

// buildBackupDateFilter constructs a BackupDateFilter from GraphQL input fields.
func buildBackupDateFilter(input *lsBackupInput) (worker.BackupDateFilter, error) {
	filter := worker.BackupDateFilter{}
	if input.LastNDays < 0 {
		return filter, errors.Errorf("lastNDays must be a positive integer, got %d", input.LastNDays)
	}
	if input.LastNDays > 0 && input.SinceDate != "" {
		return filter, errors.Errorf("lastNDays and sinceDate are mutually exclusive")
	}
	if input.LastNDays > 0 {
		since := time.Now().UTC().AddDate(0, 0, -input.LastNDays).Truncate(24 * time.Hour)
		filter.Since = &since
	}
	if input.SinceDate != "" {
		t, err := parseGraphQLDate(input.SinceDate)
		if err != nil {
			return filter, errors.Errorf("invalid sinceDate %q: %v", input.SinceDate, err)
		}
		filter.Since = &t
	}
	if input.UntilDate != "" {
		t, err := parseGraphQLDate(input.UntilDate)
		if err != nil {
			return filter, errors.Errorf("invalid untilDate %q: %v", input.UntilDate, err)
		}
		var end time.Time
		if strings.Contains(input.UntilDate, "T") {
			// RFC3339 datetime: user gave an exact timestamp, respect it.
			end = t
		} else {
			// Plain date (YYYY-MM-DD): extend to end of that calendar day.
			end = t.Add(24*time.Hour - time.Millisecond)
		}
		filter.Until = &end
	}
	return filter, nil
}

// needsFullManifest returns true when the full manifest.json must be read.
// It is true when the caller explicitly sets fullManifest OR when the query
// selection set includes "groups", so existing queries that request groups
// continue to receive populated data without any input change.
func needsFullManifest(fullManifestFlag bool, selectionSet []schema.Field) bool {
	if fullManifestFlag {
		return true
	}
	for _, f := range selectionSet {
		if f.Name() == "groups" {
			return true
		}
	}
	return false
}

func resolveListBackups(ctx context.Context, q schema.Query) *resolve.Resolved {
	input, err := getLsBackupInput(q)
	if err != nil {
		return resolve.EmptyResult(q, err)
	}

	filter, err := buildBackupDateFilter(input)
	if err != nil {
		return resolve.EmptyResult(q, err)
	}

	creds := &x.MinioCredentials{
		AccessKey:    input.AccessKey,
		SecretKey:    input.SecretKey,
		SessionToken: input.SessionToken,
		Anonymous:    input.Anonymous,
	}
	manifests, err := worker.ProcessListBackups(ctx, input.Location, creds,
		needsFullManifest(input.FullManifest, q.SelectionSet()))
	if err != nil {
		return resolve.EmptyResult(q, errors.Errorf("%s: %s", x.Error, err.Error()))
	}
	manifests = worker.FilterManifestsByDate(manifests, filter)

	convertedManifests := convertManifests(manifests)

	results := make([]map[string]interface{}, 0)
	for _, m := range convertedManifests {
		b, err := json.Marshal(m)
		if err != nil {
			return resolve.EmptyResult(q, err)
		}
		var result map[string]interface{}
		err = schema.Unmarshal(b, &result)
		if err != nil {
			return resolve.EmptyResult(q, err)
		}
		results = append(results, result)
	}

	return resolve.DataResult(
		q,
		map[string]interface{}{q.Name(): results},
		nil,
	)
}

func getLsBackupInput(q schema.Query) (*lsBackupInput, error) {
	inputArg := q.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input lsBackupInput
	err = json.Unmarshal(inputByts, &input)
	return &input, schema.GQLWrapf(err, "couldn't get input argument")
}

func convertManifests(manifests []*worker.Manifest) []*manifest {
	res := make([]*manifest, len(manifests))
	for i, m := range manifests {
		res[i] = &manifest{
			Type:      m.Type,
			Since:     m.SinceTsDeprecated,
			ReadTs:    m.ReadTs,
			BackupId:  m.BackupId,
			BackupNum: m.BackupNum,
			Path:      m.Path,
			Encrypted: m.Encrypted,
		}

		res[i].Groups = make([]*group, 0)
		for gid, preds := range m.Groups {
			res[i].Groups = append(res[i].Groups, &group{
				GroupId:    gid,
				Predicates: preds,
			})
		}
	}
	return res
}
