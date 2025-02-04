/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

// Package upgrade provides the upgrade functionality which can be used while upgrading dgraph to a
// new version from an old version.
//
// The code in this package is very much dependent on dgraph version. Please be very careful while
// modifying any files in this package. It is expected that only new files will be added over
// time in this package, and any change in existing files in not expected, as they would have been
// correct for the version of dgraph for which they were introduced. So, please double check whether
// you really need to modify any existing files.
//
// When adding upgrade capability for a new dgraph release version, follow these steps:
//  1. Create a file named `change_<release_version>.go`. For example: change_v20.07.0.go
//  2. For any change that needs to be introduced in that version, create a function of the form
//     `func() error` that applies that change, in the newly created file.
//  3. Add that function to change_list.go inside the changes for the change set introduced in
//     that version. Also add a short name and some meaningful description with it.
//
// Points to keep in mind:
//  1. Upgrade is expected only for breaking changes which go in as part of the breaking releases.
//  2. Look at the upgrade algorithm in upgrade.go to understand how & when a change is applied.
//  3. There are many re-usable functions in utils.go for the upgrade process, look at them too.
//  4. Thoroughly test your upgrade to make sure that it works correctly while upgrading from
//     previous versions to the new release version, as an upgrade is very critical process.
package upgrade
