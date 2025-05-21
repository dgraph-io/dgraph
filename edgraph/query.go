/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"
	"fmt"

	"github.com/dgraph-io/dgo/v250/protos/api"
	apiv2 "github.com/dgraph-io/dgo/v250/protos/api.v2"
	"github.com/hypermodeinc/dgraph/v25/dql"
	"github.com/hypermodeinc/dgraph/v25/x"

	"google.golang.org/grpc/status"
)

func (s *ServerV25) Ping(ctx context.Context, req *apiv2.PingRequest) (*apiv2.PingResponse, error) {
	if err := x.HealthCheck(); err != nil {
		return nil, err
	}

	return &apiv2.PingResponse{Version: x.Version()}, nil
}

// Alter handles requests to change the schema or remove parts or all of the data.
func (s *ServerV25) RunDQL(ctx context.Context, req *apiv2.RunDQLRequest) (*apiv2.RunDQLResponse, error) {
	// For now, we only allow guardian of galaxies to do this operation in v25
	if err := AuthSuperAdmin(ctx); err != nil {
		s := status.Convert(err)
		return nil, status.Error(s.Code(),
			"v25.RunDQL can only be called by the guardian of the galaxy. "+s.Message())
	}

	nsID, err := getNamespaceIDFromName(x.AttachJWTNamespace(ctx), req.NsName)
	if err != nil {
		return nil, fmt.Errorf("error retrieving namespace ID: %w", err)
	}

	apiReq, err := dql.ParseDQL(req.DqlQuery)
	if err != nil {
		return nil, fmt.Errorf("error parsing DQL query: %w", err)
	}

	apiReq.Vars = req.Vars
	apiReq.ReadOnly = req.ReadOnly
	apiReq.BestEffort = req.BestEffort
	apiReq.RespFormat = api.Request_JSON
	if req.RespFormat == apiv2.RespFormat_RDF {
		apiReq.RespFormat = api.Request_RDF
	}
	if len(apiReq.Mutations) > 0 {
		apiReq.CommitNow = true
	}

	apiResp, err := (&Server{}).doQuery(x.AttachNamespace(ctx, nsID),
		&Request{req: apiReq, doAuth: NoAuthorize})
	if err != nil {
		return nil, err
	}

	resp := &apiv2.RunDQLResponse{
		Txn: &apiv2.TxnContext{
			StartTs:  apiResp.Txn.StartTs,
			CommitTs: apiResp.Txn.CommitTs,
			Aborted:  apiResp.Txn.Aborted,
			Keys:     apiResp.Txn.Keys,
			Preds:    apiResp.Txn.Preds,
			Hash:     apiResp.Txn.Hash,
		},
		BlankUids: apiResp.Uids,
		Latency: &apiv2.Latency{
			ParsingNs:         apiResp.Latency.ParsingNs,
			ProcessingNs:      apiResp.Latency.ProcessingNs,
			RespEncodingNs:    apiResp.Latency.EncodingNs,
			AssignTimestampNs: apiResp.Latency.AssignTimestampNs,
			TotalNs:           apiResp.Latency.TotalNs,
		},
		Metrics: &apiv2.Metrics{
			UidsTouched: apiResp.Metrics.NumUids,
		},
	}

	if req.RespFormat == apiv2.RespFormat_JSON {
		resp.QueryResult = apiResp.Json
	} else if req.RespFormat == apiv2.RespFormat_RDF {
		resp.QueryResult = apiResp.Rdf
	}

	return resp, nil
}
