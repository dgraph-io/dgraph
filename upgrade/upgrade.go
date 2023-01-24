/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package upgrade

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dgraph-io/dgraph/x"
)

var (
	// Upgrade is the sub-command used to upgrade dgraph cluster.
	Upgrade x.SubCommand
	// zeroVersion represents &version{major: 0, minor: 0, patch: 0}
	zeroVersion = &version{}
	// allChanges contains all the changes ever introduced since the beginning of dgraph in the form
	// of a list of change sets.
	allChanges changeList
)

type versionComparisonResult uint8

const (
	dryRun    = "dry-run"
	alpha     = "alpha"
	slashGrpc = "slash_grpc_endpoint"
	authToken = "auth_token"
	alphaHttp = "alpha-http"
	user      = "user"
	password  = "password"
	deleteOld = "deleteOld"
	from      = "from"
	to        = "to"

	versionFmtBeforeCalVer = "v%d.%d.%d"
	versionFmtAfterCalVer  = "v%d.%02d.%d"

	less versionComparisonResult = iota
	equal
	greater
)

// version represents a mainstream release version
// for eg: for v20.03.1, major = 20, minor = 3, patch = 1
type version struct {
	major int
	minor int
	patch int
}

func (v *version) String() string {
	if v == nil {
		v = zeroVersion
	}

	versionFmt := versionFmtAfterCalVer
	if v.major < 20 {
		versionFmt = versionFmtBeforeCalVer
	}

	return fmt.Sprintf(versionFmt, v.major, v.minor, v.patch)
}

// Compare compares v with other version, and tells if v is equal, less, or greater than other.
// It interprets nil as &version{major: 0, minor: 0, patch: 0}
func (v *version) Compare(other *version) versionComparisonResult {
	if v == nil {
		v = zeroVersion
	}
	if other == nil {
		other = zeroVersion
	}

	switch {
	case v.major > other.major:
		return greater
	case v.major < other.major:
		return less
	case v.minor > other.minor:
		return greater
	case v.minor < other.minor:
		return less
	case v.patch > other.patch:
		return greater
	case v.patch < other.patch:
		return less
	}

	return equal
}

// change represents the action that needs to be taken during upgrade as a result of some breaking
// change
type change struct {
	name        string // a short name to identify the change
	description string // longer description
	// The minimum from version, upgrading from which this change can be applied.
	// For a version less than this value, this change is not applicable
	minFromVersion *version
	applyFunc      func() error // function which applies the change
}

// changeSet represents a set of changes that were introduced in some version
type changeSet struct {
	introducedIn *version // the version in which the changes were introduced
	// the list of changes that were introduced in the above version
	// it should contain changes in sorted order, based on their order of introduction as they would
	// be applied in the order they are present in this list
	changes []*change
}

// changeList represents a list of changeSet, i.e., a list of all the changes that were ever
// introduced since the beginning of Dgraph.
// The changeSets in this list are supposed to be in sorted order based on when they were introduced.
type changeList []*changeSet

type commandInput struct {
	fromVersion *version
	toVersion   *version
}

func init() {
	Upgrade.Cmd = &cobra.Command{
		Use:   "upgrade",
		Short: "Run the Dgraph upgrade tool",
		Long: "This tool is supported only for the mainstream release versions of Dgraph, " +
			"not for the beta releases.",
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
		Annotations: map[string]string{"group": "tool"},
	}
	Upgrade.EnvPrefix = "DGRAPH_UPGRADE"
	Upgrade.Cmd.SetHelpTemplate(x.NonRootTemplate)
	flag := Upgrade.Cmd.Flags()
	flag.Bool(dryRun, false, "dry-run the upgrade")
	flag.StringP(alpha, "a", "127.0.0.1:9080",
		"Comma separated list of Dgraph Alpha gRPC server address")
	flag.String(slashGrpc, "", "Path to Slash GraphQL GRPC endpoint. "+
		"If --slash_grpc_endpoint is set, all other TLS options and connection options will be "+
		"ignored")
	flag.String(authToken, "",
		"The auth token passed to the server for Alter operation of the schema file. "+
			"If used with --slash_grpc_endpoint, then this should be set to the API token issued"+
			"by Slash GraphQL")
	flag.String(alphaHttp, "http://127.0.0.1:8080", "Draph Alpha HTTP(S) endpoint.")
	flag.StringP(user, "u", "", "Username of ACL user")
	flag.StringP(password, "p", "", "Password of ACL user")
	flag.BoolP(deleteOld, "d", true, "Delete the older ACL types/predicates")
	flag.StringP(from, "f", "", "The version string from which to upgrade, e.g.: v1.2.2")
	flag.StringP(to, "t", "", "The version string till which to upgrade, e.g.: v20.03.0")

	x.RegisterClientTLSFlags(flag)
}

func run() {
	cmdInput, err := validateAndParseInput()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	// Login using dgo client fetches the information from creds flag.
	Upgrade.Conf.Set("creds", fmt.Sprintf("user=%s; password=%s; namespace=%d",
		Upgrade.Conf.GetString(user), Upgrade.Conf.GetString(password), x.GalaxyNamespace))
	applyChangeList(cmdInput, allChanges)
}

func validateAndParseInput() (*commandInput, error) {
	_, _, err := net.SplitHostPort(strings.TrimSpace(Upgrade.Conf.GetString(alpha)))
	if err != nil {
		return nil, formatAsFlagParsingError(alpha, err)
	}

	fromVersionParsed, err := parseVersionFromString(Upgrade.Conf.GetString(from))
	if err != nil {
		return nil, formatAsFlagParsingError(from, err)
	}

	toVersionParsed, err := parseVersionFromString(Upgrade.Conf.GetString(to))
	if err != nil {
		return nil, formatAsFlagParsingError(to, err)
	}

	if fromVersionParsed.Compare(toVersionParsed) != less {
		return nil, fmt.Errorf("error: `%s` must be less than `%s`", from, to)
	}

	return &commandInput{fromVersion: fromVersionParsed, toVersion: toVersionParsed}, nil
}

func formatAsFlagParsingError(flag string, err error) error {
	return fmt.Errorf("error parsing flag `%s`: %w", flag, err)
}

// parseVersionFromString parses a version given as string to internal representation.
// Some examples for input and output:
//  1. input : v1.2.2
//     output: &version{major: 1, minor: 2, patch: 2}, nil
//  2. input : v20.03.0-beta.20200320
//     output: &version{major: 20, minor: 3, patch: 0}, nil
//  3. input : 1.2.2
//     output: nil, error
//  4. input : v1.2.2s
//     output: nil, error
func parseVersionFromString(v string) (*version, error) {
	v = strings.TrimSpace(v)
	if v == "" || v[:1] != "v" {
		return nil, fmt.Errorf("version can't be empty and must start with `v`. E.g.: v1.2.2")
	}

	versionSplit := strings.Split(v[1:], ".")
	result := &version{}
	var err error

	result.major, err = strconv.Atoi(versionSplit[0])
	if err != nil {
		return nil, fmt.Errorf("error parsing major version: %w", err)
	}

	result.minor, err = strconv.Atoi(versionSplit[1])
	if err != nil {
		return nil, fmt.Errorf("error parsing minor version: %w", err)
	}

	// the third part of split might contain some extra things like `beta` appended with -,
	// so split again and take the first part as patch
	patchSplit := strings.Split(versionSplit[2], "-")
	result.patch, err = strconv.Atoi(patchSplit[0])
	if err != nil {
		return nil, fmt.Errorf("error parsing patch version: %w", err)
	}

	return result, nil
}

func applyChangeList(cmdInput *commandInput, list changeList) {
	fmt.Println("**********************************************")
	fmt.Println("Version to upgrade from:", cmdInput.fromVersion)
	fmt.Println("Version to upgrade to  :", cmdInput.toVersion)
	fmt.Println("**********************************************")
	// sort the changeList based on the version the changes were introduced in
	sort.SliceStable(list, func(i, j int) bool {
		iVersion := list[i].introducedIn
		jVersion := list[j].introducedIn
		return iVersion.Compare(jVersion) == less
	})

	isInitialFromVersion := true

	for j, changeSet := range list {
		// apply the change set only if the version in which it was introduced is <= the version
		// to which we want to upgrade and also >= the version from which we want to upgrade.
		// If cmdInput.fromVersion hasn't been changed by us yet, then make sure that it is always
		// less than the introduced version, ignore equality. So, if upgrading from version x to
		// some other version, then the changes introduced in version x don't get applied.
		fromConditionSatisfied := false
		if isInitialFromVersion {
			fromConditionSatisfied = cmdInput.fromVersion.Compare(changeSet.introducedIn) == less
		} else {
			fromConditionSatisfied = cmdInput.fromVersion.Compare(changeSet.introducedIn) != greater
		}
		if cmdInput.toVersion.Compare(changeSet.introducedIn) != less && fromConditionSatisfied {
			fmt.Printf("\n%d. Applying the change set introduced in version: %s\n", j+1,
				changeSet.introducedIn)
			// Go over every change in the change set and check if it should be applied. If yes,
			// apply it, otherwise skip it.
			for i, change := range changeSet.changes {
				fmt.Println(fmt.Sprintf(""+
					"\tApplying change %d:\n"+
					"\t\tName       : %s\n"+
					"\t\tDescription: %s", i+1, change.name, change.description))
				// apply the change only if the min version from which it should be applied
				// is <= the version from which we want to upgrade
				if cmdInput.fromVersion.Compare(change.minFromVersion) != less {
					var err error
					if !Upgrade.Conf.GetBool(dryRun) {
						err = change.applyFunc()
					}
					if err != nil {
						fmt.Println("\t\tStatus     : Error")
						fmt.Println("\t\tError      :", err)
						fmt.Println("\t\tCan't continue. Exiting!!!")
						os.Exit(1)
					} else {
						fmt.Println("\t\tStatus     : Successful")
					}
				} else {
					fmt.Println("\t\tStatus     : Skipped (min version mismatch)")
				}
			}
			// update the fromVersion to reflect the version till which DB has been upgraded,
			// so that minFromVersion check is satisfied for the changes in next changeSets
			cmdInput.fromVersion = changeSet.introducedIn
			isInitialFromVersion = false
			fmt.Println("\tDB state now upgraded to:", cmdInput.fromVersion)
		} else {
			fmt.Println("Skipping the change set introduced in version:", changeSet.introducedIn)
		}
	}

	fmt.Println("\nUpgrade finished!")
}
