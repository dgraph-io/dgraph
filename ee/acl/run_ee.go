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
	"time"

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

var opt options
var tlsConf x.TLSHelperConfig

var CmdAcl x.SubCommand

func init() {
	CmdAcl.Cmd = &cobra.Command{
		Use:   "acl",
		Short: "Run the Dgraph acl tool",
	}

	flag := CmdAcl.Cmd.PersistentFlags()
	flag.StringP("dgraph", "d", "127.0.0.1:9080", "Dgraph gRPC server address")

	// TLS configuration
	x.RegisterTLSFlags(flag)
	flag.String("tls_server_name", "", "Used to verify the server hostname.")

	subcommands := initSubcommands()
	for _, sc := range subcommands {
		CmdAcl.Cmd.AddCommand(sc.Cmd)
		sc.Conf = viper.New()
		if err := sc.Conf.BindPFlags(sc.Cmd.Flags()); err != nil {
			glog.Fatalf("Unable to bind flags for command %v:%v", sc, err)
		}
		if err := sc.Conf.BindPFlags(CmdAcl.Cmd.PersistentFlags()); err != nil {
			glog.Fatalf("Unable to bind persistent flags from acl for command %v:%v", sc, err)
		}
		sc.Conf.SetEnvPrefix(sc.EnvPrefix)
	}
}

func initSubcommands() []*x.SubCommand {
	// user creation command
	var cmdUserAdd x.SubCommand
	cmdUserAdd.Cmd = &cobra.Command{
		Use:   "useradd",
		Short: "Run Dgraph acl tool to add a user",
		Run: func(cmd *cobra.Command, args []string) {
			if err := userAdd(cmdUserAdd.Conf); err != nil {
				glog.Errorf("Unable to add user:%v", err)
				os.Exit(1)
			}
		},
	}
	userAddFlags := cmdUserAdd.Cmd.Flags()
	userAddFlags.StringP("user", "u", "", "The user id to be created")
	userAddFlags.StringP("password", "p", "", "The password for the user")

	// user deletion command
	var cmdUserDel x.SubCommand
	cmdUserDel.Cmd = &cobra.Command{
		Use:   "userdel",
		Short: "Run Dgraph acl tool to delete a user",
		Run: func(cmd *cobra.Command, args []string) {
			if err := userDel(cmdUserDel.Conf); err != nil {
				glog.Errorf("Unable to delete the user:%v", err)
				os.Exit(1)
			}
		},
	}
	userDelFlags := cmdUserDel.Cmd.Flags()
	userDelFlags.StringP("user", "u", "", "The user id to be deleted")

	// login command
	var cmdLogIn x.SubCommand
	cmdLogIn.Cmd = &cobra.Command{
		Use:   "login",
		Short: "Login to dgraph in order to get a jwt token",
		Run: func(cmd *cobra.Command, args []string) {
			if err := userLogin(cmdLogIn.Conf); err != nil {
				glog.Errorf("Unable to login:%v", err)
				os.Exit(1)
			}
		},
	}
	loginFlags := cmdLogIn.Cmd.Flags()
	loginFlags.StringP("user", "u", "", "The user id to be created")
	loginFlags.StringP("password", "p", "", "The password for the user")

	// group creation command
	var cmdGroupAdd x.SubCommand
	cmdGroupAdd.Cmd = &cobra.Command{
		Use:   "groupadd",
		Short: "Run Dgraph acl tool to add a group",
		Run: func(cmd *cobra.Command, args []string) {
			if err := groupAdd(cmdGroupAdd.Conf); err != nil {
				glog.Errorf("Unable to add group:%v", err)
				os.Exit(1)
			}
		},
	}
	groupAddFlags := cmdGroupAdd.Cmd.Flags()
	groupAddFlags.StringP("group", "g", "", "The group id to be created")

	// group deletion command
	var cmdGroupDel x.SubCommand
	cmdGroupDel.Cmd = &cobra.Command{
		Use:   "groupdel",
		Short: "Run Dgraph acl tool to delete a group",
		Run: func(cmd *cobra.Command, args []string) {
			if err := groupDel(cmdGroupDel.Conf); err != nil {
				glog.Errorf("Unable to delete group:%v", err)
				os.Exit(1)
			}
		},
	}
	groupDelFlags := cmdGroupDel.Cmd.Flags()
	groupDelFlags.StringP("group", "g", "", "The group id to be deleted")

	// the usermod command used to set a user's groups
	var cmdUserMod x.SubCommand
	cmdUserMod.Cmd = &cobra.Command{
		Use:   "usermod",
		Short: "Run Dgraph acl tool to change a user's groups",
		Run: func(cmd *cobra.Command, args []string) {
			if err := userMod(cmdUserMod.Conf); err != nil {
				glog.Errorf("Unable to modify user:%v", err)
				os.Exit(1)
			}
		},
	}
	userModFlags := cmdUserMod.Cmd.Flags()
	userModFlags.StringP("user", "u", "", "The user id to be changed")
	userModFlags.StringP("groups", "g", "", "The groups to be set for the user")

	// the chmod command is used to change a group's permissions
	var cmdChMod x.SubCommand
	cmdChMod.Cmd = &cobra.Command{
		Use:   "chmod",
		Short: "Run Dgraph acl tool to change a group's permissions",
		Run: func(cmd *cobra.Command, args []string) {
			if err := chMod(cmdChMod.Conf); err != nil {
				glog.Errorf("Unable to change permisson for group:%v", err)
				os.Exit(1)
			}
		},
	}
	chModFlags := cmdChMod.Cmd.Flags()
	chModFlags.StringP("group", "g", "", "The group whose permission "+
		"is to be changed")
	chModFlags.StringP("pred", "p", "", "The predicates whose acls"+
		" are to be changed")
	chModFlags.IntP("perm", "P", 0, "The acl represented using "+
		"an integer, 4 for read-only, 2 for write-only, and 1 for modify-only")

	var cmdInfo x.SubCommand
	cmdInfo.Cmd = &cobra.Command{
		Use:   "info",
		Short: "Show info about a user or group",
		Run: func(cmd *cobra.Command, args []string) {
			if err := info(cmdInfo.Conf); err != nil {
				glog.Errorf("Unable to show info:%v", err)
				os.Exit(1)
			}
		},
	}
	infoFlags := cmdInfo.Cmd.Flags()
	infoFlags.StringP("user", "u", "", "The user to be shown")
	infoFlags.StringP("group", "g", "", "The group to be shown")
	return []*x.SubCommand{
		&cmdUserAdd, &cmdUserDel, &cmdLogIn, &cmdGroupAdd, &cmdGroupDel, &cmdUserMod,
		&cmdChMod, &cmdInfo,
	}
}

type CloseFunc func()

func getDgraphClient(conf *viper.Viper) (*dgo.Dgraph, CloseFunc) {
	opt = options{
		dgraph: conf.GetString("dgraph"),
	}
	glog.Infof("Running transaction with dgraph endpoint: %v", opt.dgraph)

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
			glog.Errorf("Error while closing connection:%v", err)
		}
	}
}

func info(conf *viper.Viper) error {
	userId := conf.GetString("user")
	groupId := conf.GetString("group")
	if (len(userId) == 0 && len(groupId) == 0) ||
		(len(userId) != 0 && len(groupId) != 0) {
		return fmt.Errorf("either the user or group should be specified, not both")
	}

	dc, close := getDgraphClient(conf)
	defer close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	txn := dc.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			glog.Errorf("Unable to discard transaction:%v", err)
		}
	}()

	if len(userId) != 0 {
		user, err := queryUser(ctx, txn, userId)
		if err != nil {
			return err
		}

		var userBuf strings.Builder
		userBuf.WriteString(fmt.Sprintf("user %v:\n", userId))
		userBuf.WriteString(fmt.Sprintf("uid:%v\nid:%v\n", user.Uid, user.UserID))
		var groupNames []string
		for _, group := range user.Groups {
			groupNames = append(groupNames, group.GroupID)
		}
		userBuf.WriteString(fmt.Sprintf("groups:%v\n", strings.Join(groupNames, " ")))
		glog.Infof(userBuf.String())
	}

	if len(groupId) != 0 {
		group, err := queryGroup(ctx, txn, groupId, "dgraph.xid", "~dgraph.user.group{dgraph.xid}",
			"dgraph.group.acl")
		if err != nil {
			return err
		}
		// build the info string for group
		var groupSB strings.Builder
		groupSB.WriteString(fmt.Sprintf("group %v:\n", groupId))
		groupSB.WriteString(fmt.Sprintf("uid:%v\nid:%v\n", group.Uid, group.GroupID))

		var userNames []string
		for _, user := range group.Users {
			userNames = append(userNames, user.UserID)
		}
		groupSB.WriteString(fmt.Sprintf("users:%v\n", strings.Join(userNames, " ")))

		var aclStrs []string
		var acls []Acl
		if err := json.Unmarshal([]byte(group.Acls), &acls); err != nil {
			return fmt.Errorf("unable to unmarshal the acls associated with the group %v:%v",
				groupId, err)
		}

		for _, acl := range acls {
			aclStrs = append(aclStrs, fmt.Sprintf("(predicate:%v,perm:%v)", acl.Predicate, acl.Perm))
		}
		groupSB.WriteString(fmt.Sprintf("acls:%v\n", strings.Join(aclStrs, " ")))

		glog.Infof(groupSB.String())
	}

	return nil
}
