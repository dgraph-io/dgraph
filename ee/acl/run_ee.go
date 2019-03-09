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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type options struct {
	dgraph string
}

var (
	opt     options
	tlsConf x.TLSHelperConfig
	CmdAcl  x.SubCommand
)

const gPassword = "gpassword"

func init() {
	CmdAcl.Cmd = &cobra.Command{
		Use:   "acl",
		Short: "Run the Dgraph acl tool",
	}

	flag := CmdAcl.Cmd.PersistentFlags()
	flag.StringP("dgraph", "d", "127.0.0.1:9080", "Dgraph Alpha gRPC server address")
	flag.StringP(gPassword, "x", "", "Groot password to authorize this operation")

	// TLS configuration
	x.RegisterTLSFlags(flag)
	flag.String("tls_server_name", "", "Used to verify the server hostname.")

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
	modFlags.StringP("new_password", "", "", "The new password for the user")
	modFlags.StringP("group_list", "l", "", "The list of groups to be set for the user")
	modFlags.StringP("group", "g", "", "The group whose permission is to be changed")
	modFlags.StringP("pred", "p", "", "The predicates whose acls are to be changed")
	modFlags.StringP("pred_regex", "", "", "The regular expression specifying predicates"+
		" whose acls are to be changed")
	modFlags.IntP("perm", "P", 0, "The acl represented using "+
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

type CloseFunc func()

func getDgraphClient(conf *viper.Viper) (*dgo.Dgraph, CloseFunc) {
	opt = options{
		dgraph: conf.GetString("dgraph"),
	}
	fmt.Printf("\nRunning transaction with dgraph endpoint: %v\n", opt.dgraph)

	if len(opt.dgraph) == 0 {
		glog.Fatalf("The --dgraph option must be set in order to connect to dgraph")
	}

	x.LoadTLSConfig(&tlsConf, CmdAcl.Conf, x.TlsClientCert, x.TlsClientKey)
	tlsConf.ServerName = CmdAcl.Conf.GetString("tls_server_name")

	conn, err := x.SetupConnection(opt.dgraph, &tlsConf, false)
	x.Checkf(err, "While trying to setup connection to Dgraph alpha.")

	dc := api.NewDgraphClient(conn)
	return dgo.NewDgraphClient(dc), func() {
		if err := conn.Close(); err != nil {
			fmt.Printf("Error while closing connection: %v\n", err)
		}
	}
}

func queryAndPrintUser(ctx context.Context, txn *dgo.Txn, userId string) error {
	user, err := queryUser(ctx, txn, userId)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("The user %q does not exist.\n", userId)
	}

	fmt.Printf("User  : %s\n", userId)
	fmt.Printf("UID   : %s\n", user.Uid)
	for _, group := range user.Groups {
		fmt.Printf("Group : %-5s\n", group.GroupID)
	}
	return nil
}

func queryAndPrintGroup(ctx context.Context, txn *dgo.Txn, groupId string) error {
	group, err := queryGroup(ctx, txn, groupId, "dgraph.xid", "~dgraph.user.group{dgraph.xid}",
		"dgraph.group.acl")
	if err != nil {
		return err
	}
	if group == nil {
		return fmt.Errorf("The group %q does not exist.\n", groupId)
	}
	fmt.Printf("Group: %s\n", groupId)
	fmt.Printf("UID  : %s\n", group.Uid)
	fmt.Printf("ID   : %s\n", group.GroupID)

	var userNames []string
	for _, user := range group.Users {
		userNames = append(userNames, user.UserID)
	}
	fmt.Printf("Users: %s\n", strings.Join(userNames, " "))

	var acls []Acl
	if len(group.Acls) != 0 {
		if err := json.Unmarshal([]byte(group.Acls), &acls); err != nil {
			return fmt.Errorf("unable to unmarshal the acls associated with the group %v: %v",
				groupId, err)
		}

		for _, acl := range acls {
			fmt.Printf("ACL  : %v\n", acl)
		}
	}
	return nil
}

func info(conf *viper.Viper) error {
	userId, groupId, err := getUserAndGroup(conf)
	if err != nil {
		return err
	}

	dc, cancel, err := getClientWithAdminCtx(conf)
	defer cancel()
	if err != nil {
		return fmt.Errorf("unable to get admin context: %v\n", err)
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			fmt.Printf("Unable to discard transaction: %v\n", err)
		}
	}()

	if len(userId) != 0 {
		return queryAndPrintUser(ctx, txn, userId)
	}

	return queryAndPrintGroup(ctx, txn, groupId)
}
