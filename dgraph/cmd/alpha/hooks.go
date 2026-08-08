/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package alpha

import (
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
)

// This file declares public extensibility hooks for the Alpha gRPC server.
// Each hook is a package-level function var with an upstream-safe no-op
// default; the fork reassigns them from dgraph/cmd/alpha/init_istari.go.

// RegisterFlags registers fork-specific CLI flags onto f. Called from
// Alpha's init() after Alpha.Cmd is initialized, so the FlagSet is live.
// Default: no-op (OSS builds).
var RegisterFlags = defaultRegisterFlags

func defaultRegisterFlags(f *pflag.FlagSet) {}

// RegisterZanzibar wires the Zanzibar gRPC service onto s and bootstraps the
// fixed predicate schema. Default: no-op (OSS builds).
var RegisterZanzibar = defaultRegisterZanzibar

func defaultRegisterZanzibar(s *grpc.Server) {}

// ZanzibarUnaryInterceptor returns a unary server interceptor for Zanzibar
// auth gating, or nil if Zanzibar is not enabled. Default: returns nil.
var ZanzibarUnaryInterceptor = defaultZanzibarUnaryInterceptor

func defaultZanzibarUnaryInterceptor() grpc.UnaryServerInterceptor { return nil }

// ZanzibarStreamInterceptor is the streaming counterpart to
// ZanzibarUnaryInterceptor. Default: returns nil.
var ZanzibarStreamInterceptor = defaultZanzibarStreamInterceptor

func defaultZanzibarStreamInterceptor() grpc.StreamServerInterceptor { return nil }
