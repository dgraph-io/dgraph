/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import "sync"

// Options contains options for the postings package.
type Options struct {
	sync.Mutex

	CommitFraction float64
}

// Config stores the posting options of this instance.
var Config Options
