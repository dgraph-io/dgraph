package admin

import (
	"context"
	"encoding/json"

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

type lsBackupInput struct {
	Location     string
	AccessKey    string
	SecretKey    string
	SessionToken string
	Anonymous    bool
	ForceFull    bool
}

type group struct {
	GroupId    uint32   `json:"groupId,omitempty"`
	Predicates []string `json:"predicates,omitempty"`
}

type manifest struct {
	Type      string   `json:"type,omitempty"`
	Since     uint64   `json:"since,omitempty"`
	Groups    []*group `json:"groups,omitempty"`
	BackupId  string   `json:"backupId,omitempty"`
	BackupNum uint64   `json:"backupNum,omitempty"`
	Path      string   `json:"path,omitempty"`
	Encrypted bool     `json:"encrypted,omitempty"`
}

func resolveListBackups(ctx context.Context, q schema.Query) *resolve.Resolved {
	input, err := getLsBackupInput(q)
	if err != nil {
		return emptyResult(q, err)
	}

	creds := &worker.Credentials{
		AccessKey:    input.AccessKey,
		SecretKey:    input.SecretKey,
		SessionToken: input.SessionToken,
		Anonymous:    input.Anonymous,
	}
	manifests, err := worker.ProcessListBackups(ctx, input.Location, creds)
	if err != nil {
		return emptyResult(q, errors.Errorf("%s: %s", x.Error, err.Error()))
	}
	convertedManifests := convertManifests(manifests)

	results := make([]map[string]interface{}, 0)
	for _, m := range convertedManifests {
		b, err := json.Marshal(m)
		if err != nil {
			return emptyResult(q, err)
		}
		var result map[string]interface{}
		err = json.Unmarshal(b, &result)
		if err != nil {
			return emptyResult(q, err)
		}
		results = append(results, result)
	}

	return &resolve.Resolved{
		Data:  map[string]interface{}{q.Name(): results},
		Field: q,
	}
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
			Since:     m.Since,
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
