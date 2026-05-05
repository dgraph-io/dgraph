/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

// This file declares public extensibility hooks for the compose generator.
// See testutil/hooks.go for the full convention.

// EmitUser returns the value for the generated `user:` field in docker
// compose services. Default: empty string, emitting no user override
// and matching upstream behavior where containers run as the image's
// default user. Forks may return, e.g., "${UID:-65532}" to pin services
// to the host UID for bind-mount compatibility under nonroot runtime
// images.
var EmitUser = defaultEmitUser

func defaultEmitUser() string { return "" }
