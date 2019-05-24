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

package acl

import (
	"fmt"
	"os"

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type options struct {
	alpha string
}

var (
	opt options
	// CmdAcl is the sub-command used to manage the ACL system.
	CmdAcl x.SubCommand
)

const gPassword = "gpassword"
const defaultGroupList = "dgraph-unused-group"

func init() {
	CmdAcl.Cmd = &cobra.Command{
		Use:   "acl",
		Short: "Run the Dgraph acl tool",
	}

	flag := CmdAcl.Cmd.PersistentFlags()
	flag.StringP("alpha", "a", "127.0.0.1:9080", "Dgraph Alpha gRPC server address")
	flag.StringP(gPassword, "x", "", "Groot password to authorize this operation")

	// TLS configuration
	x.RegisterClientTLSFlags(flag)

	subcommands := initSubcommands()
	for _, sc := range subcommands {
		CmdAcl.Cmd.AddCommand(sc.Cmd)
		sc.Conf = viper.New()
		if err := sc.Conf.BindPFlags(sc.Cmd.Flags()); err != nil {
			glog.Fatalf("Unable to bind flags for command %v: %v", sc, err)
		}
		if err := sc.Conf.BindPFlags(CmdAcl.Cmd.PersistentFlags()); err != nil {
			glog.Fatalf("Unable to bind persistent flags from acl for command %v: %v", sc, err)
		}
		sc.Conf.SetEnvPrefix(sc.EnvPrefix)
	}
}

func initSubcommands() []*x.SubCommand {
	var cmdAdd x.SubCommand
	cmdAdd.Cmd = &cobra.Command{
		Use:   "add",
		Short: "Run Dgraph acl tool to add a user or group",
		Run: func(cmd *cobra.Command, args []string) {
			if err := add(cmdAdd.Conf); err != nil {
				fmt.Printf("%v\n", err)
				os.Exit(1)
			}
		},
	}

	addFlags := cmdAdd.Cmd.Flags()
	addFlags.StringP("user", "u", "", "The user id to be created")
	addFlags.StringP("password", "p", "", "The password for the user")
	addFlags.StringP("group", "g", "", "The group id to be created")

	var cmdDel x.SubCommand
	cmdDel.Cmd = &cobra.Command{
		Use:   "del",
		Short: "Run Dgraph acl tool to delete a user or group",
		Run: func(cmd *cobra.Command, args []string) {
			if err := del(cmdDel.Conf); err != nil {
				fmt.Printf("Unable to delete the user: %v\n", err)
				os.Exit(1)
			}
		},
	}

	delFlags := cmdDel.Cmd.Flags()
	delFlags.StringP("user", "u", "", "The user id to be deleted")
	delFlags.StringP("group", "g", "", "The group id to be deleted")

	var cmdMod x.SubCommand
	cmdMod.Cmd = &cobra.Command{
		Use: "mod",
		Short: "Run Dgraph acl tool to modify a user's password, a user's group list, or a" +
			"group's predicate permissions",
		Run: func(cmd *cobra.Command, args []string) {
			if err := mod(cmdMod.Conf); err != nil {
				fmt.Printf("Unable to modify: %v\n", err)
				os.Exit(1)
			}
		},
	}

	modFlags := cmdMod.Cmd.Flags()
	modFlags.StringP("user", "u", "", "The user id to be changed")
	modFlags.BoolP("new_password", "n", false, "Whether to reset password for the user")
	modFlags.StringP("group_list", "l", defaultGroupList,
		"The list of groups to be set for the user")
	modFlags.StringP("group", "g", "", "The group whose permission is to be changed")
	modFlags.StringP("pred", "p", "", "The predicates whose acls are to be changed")
	modFlags.StringP("pred_regex", "P", "", "The regular expression specifying predicates"+
		" whose acls are to be changed")
	modFlags.IntP("perm", "m", 0, "The acl represented using "+
		"an integer: 4 for read, 2 for write, and 1 for modify. Use a negative value to remove a "+
		"predicate from the group")

	var cmdInfo x.SubCommand
	cmdInfo.Cmd = &cobra.Command{
		Use:   "info",
		Short: "Show info about a user or group",
		Run: func(cmd *cobra.Command, args []string) {
			if err := info(cmdInfo.Conf); err != nil {
				fmt.Printf("Unable to show info: %v\n", err)
				os.Exit(1)
			}
		},
	}
	infoFlags := cmdInfo.Cmd.Flags()
	infoFlags.StringP("user", "u", "", "The user to be shown")
	infoFlags.StringP("group", "g", "", "The group to be shown")
	return []*x.SubCommand{&cmdAdd, &cmdDel, &cmdMod, &cmdInfo}
}
