//go:build !oss
// +build !oss

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/main/licenses/DCL.txt
 */

package audit

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"os"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/dgraph-io/dgraph/x"
)

var CmdAudit x.SubCommand

func init() {
	CmdAudit.Cmd = &cobra.Command{
		Use:         "audit",
		Short:       "Dgraph audit tool",
		Annotations: map[string]string{"group": "security"},
	}
	CmdAudit.Cmd.SetHelpTemplate(x.NonRootTemplate)

	subcommands := initSubcommands()
	for _, sc := range subcommands {
		CmdAudit.Cmd.AddCommand(sc.Cmd)
		sc.Conf = viper.New()
		if err := sc.Conf.BindPFlags(sc.Cmd.Flags()); err != nil {
			glog.Fatalf("Unable to bind flags for command %v: %v", sc, err)
		}
		if err := sc.Conf.BindPFlags(CmdAudit.Cmd.PersistentFlags()); err != nil {
			glog.Fatalf(
				"Unable to bind persistent flags from audit for command %v: %v", sc, err)
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
	decFlags.String("encryption_key_file", "", "path to encrypt files.")
	return []*x.SubCommand{&decryptCmd}
}

func run() error {
	key, err := os.ReadFile(decryptCmd.Conf.GetString("encryption_key_file"))
	x.Check(err)
	if key == nil {
		return errors.New("no encryption key provided")
	}

	file, err := os.Open(decryptCmd.Conf.GetString("in"))
	x.Check(err)
	defer func() {
		if err := file.Close(); err != nil {
			glog.Warningf("error closing file: %v", err)
		}
	}()

	outfile, err := os.OpenFile(decryptCmd.Conf.GetString("out"),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	x.Check(err)
	defer func() {
		if err := outfile.Close(); err != nil {
			glog.Warningf("error closing file: %v", err)
		}
	}()
	block, err := aes.NewCipher(key)
	x.Check(err)

	stat, err := os.Stat(decryptCmd.Conf.GetString("in"))
	x.Check(err)
	if stat.Size() == 0 {
		glog.Info("audit file is empty")
		return nil
	}

	// decrypt header in audit log, verify encryption key
	var iterator int64 = 0
	iv := make([]byte, aes.BlockSize)
	length := make([]byte, 4)
	x.Check2(file.ReadAt(iv, iterator)) // get first iv
	iterator = iterator + aes.BlockSize // iterator = 16

	verificationTextLength := make([]byte, 4)
	x.Check2(file.ReadAt(verificationTextLength, iterator))
	verificationTextLengthInt64 := int64(binary.BigEndian.Uint32(verificationTextLength)) // = 11
	iterator = iterator + 4                                                               //iterator = 20

	VerificationTextCipher := make([]byte, len(x.VerificationText))
	x.Check2(file.ReadAt(VerificationTextCipher, iterator))
	iterator = iterator + verificationTextLengthInt64 // iterator = 20 + 11 = 31

	verificationTextPlain := make([]byte, len(x.VerificationText))

	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(verificationTextPlain, VerificationTextCipher)
	if string(verificationTextPlain) != x.VerificationText {
		return errors.New("invalid encryption key provided. Please check your encryption key")
	}

	//todo(joshua): we should check if we have an deprecated audit log and if yes, decrypt it

	// decrypt body of audit log
	for {
		// if its the end of data. finish decrypting
		if iterator >= stat.Size() {
			break
		}
		x.Check2(file.ReadAt(iv, iterator))
		iterator = iterator + 16

		x.Check2(file.ReadAt(length, iterator))
		lengthInt64 := int64(binary.BigEndian.Uint32(length))
		iterator = iterator + 4

		content := make([]byte, lengthInt64)
		x.Check2(file.ReadAt(content, iterator))
		iterator = iterator + lengthInt64

		stream := cipher.NewCTR(block, iv)
		stream.XORKeyStream(content, content)
		x.Check2(outfile.Write(content))
	}
	glog.Infof("Decryption of Audit file %s is Done. Decrypted file is %s",
		decryptCmd.Conf.GetString("in"),
		decryptCmd.Conf.GetString("out"))
	return nil
}
