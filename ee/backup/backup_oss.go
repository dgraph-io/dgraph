// +build oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package backup

// BadgerKeyFile - This is a copy of worker.Config.BadgerKeyFile. Need to copy because
// otherwise it results in an import cycle.
var BadgerKeyFile string = ""
