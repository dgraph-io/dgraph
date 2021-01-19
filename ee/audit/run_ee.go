// +build !oss

/*
 * Copyright 2121 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package audit

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
)

var CmdAudit x.SubCommand

func init() {
	CmdAudit.Cmd = &cobra.Command{
		Use:   "audit",
		Short: "Dgraph audit tool",
	}

	subcommands := initSubcommands()
	for _, sc := range subcommands {
		CmdAudit.Cmd.AddCommand(sc.Cmd)
		sc.Conf = viper.New()
		if err := sc.Conf.BindPFlags(sc.Cmd.Flags()); err != nil {
			glog.Fatalf("Unable to bind flags for command %v: %v", sc, err)
		}
		if err := sc.Conf.BindPFlags(CmdAudit.Cmd.PersistentFlags()); err != nil {
			glog.Fatalf("Unable to bind persistent flags from audit for command %v: %v", sc, err)
		}
		sc.Conf.SetEnvPrefix(sc.EnvPrefix)
	}
}

var decryptCmd x.SubCommand

func initSubcommands() []*x.SubCommand {
	decryptCmd.Cmd = &cobra.Command{
		Use:   "decrypt",
		Short: "Run Dgraph Audit tool to decrypt audit files",
		Run: func(cmd *cobra.Command, args []string) {
			if err := run(); err != nil {
				fmt.Printf("%v\n", err)
				os.Exit(1)
			}
		},
	}

	decFlags := decryptCmd.Cmd.Flags()
	decFlags.String("in", "", "input file that needs to decrypted.")
	decFlags.String("out", "audit_log_out.log",
		"output file to which decrypted output will be dumped.")
	enc.RegisterFlags(decFlags)
	return []*x.SubCommand{&decryptCmd}
}

func run() error {
	key, err := enc.ReadKey(decryptCmd.Conf)
	x.Check(err)
	if key == nil {
		return errors.New("no encryption key provided")
	}
	file, err := os.Open(decryptCmd.Conf.GetString("in"))
	x.Check(err)
	defer file.Close()

	outfile, err := os.OpenFile(decryptCmd.Conf.GetString("out"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0600)
	x.Check(err)
	defer outfile.Close()

	block, err := aes.NewCipher(key)
	stat, err := os.Stat(decryptCmd.Conf.GetString("in"))
	x.Check(err)

	var iterator int64 = 0
	for {
		if iterator >= stat.Size() {
			break
		}
		l := make([]byte, 4)
		_, err := file.ReadAt(l, iterator)
		x.Check(err)
		iterator = iterator + 4
		content := make([]byte, binary.BigEndian.Uint32(l))
		_, err = file.ReadAt(content, iterator)
		x.Check(err)

		iterator = iterator + int64(binary.BigEndian.Uint32(l))
		iv := make([]byte, aes.BlockSize)
		_, err = file.ReadAt(iv, iterator)
		iterator = iterator + aes.BlockSize

		stream := cipher.NewCTR(block, iv)
		stream.XORKeyStream(content, content)
		_, err = outfile.Write(content)
		x.Check(err)
	}
	return nil
}
