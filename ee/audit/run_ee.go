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

	// decrypt header in audit log to verify encryption key
	// [16]byte IV + [4]byte len(x.VerificationText) + [11]byte x.VerificationText
	decryptHeader := func() ([]byte, int64, error) {
		var iterator int64 = 0
		iv := make([]byte, aes.BlockSize)
		_, err := file.ReadAt(iv, iterator) // get first iv
		if err != nil {
			return nil, 0, err
		}
		iterator = iterator + aes.BlockSize + 4 // length of verification text encoded in uint32

		ct := make([]byte, len(x.VerificationText))
		_, err = file.ReadAt(ct, iterator)
		if err != nil {
			return nil, 0, err
		}
		iterator = iterator + int64(len(x.VerificationText))

		text := make([]byte, len(x.VerificationText))
		stream := cipher.NewCTR(block, iv)
		stream.XORKeyStream(text, ct)
		if string(text) != x.VerificationText {
			return nil, 0, errors.New("invalid encryption key provided. Please check your encryption key")
		}
		return iv, iterator, nil
	}

	// [12]byte baseIV + [4]byte len(x.VerificationTextDeprecated) + [11]byte x.VerificationTextDeprecated
	decryptHeaderDeprecated := func() ([]byte, int64, error) {
		var iterator int64 = 0
		iv := make([]byte, aes.BlockSize)
		_, err := file.ReadAt(iv, iterator)
		if err != nil {
			return nil, 0, err
		}
		iterator = iterator + aes.BlockSize

		ct := make([]byte, len(x.VerificationTextDeprecated))
		_, err = file.ReadAt(ct, iterator)
		if err != nil {
			return nil, 0, err
		}
		iterator = iterator + int64(len(x.VerificationTextDeprecated))

		text := make([]byte, len(x.VerificationTextDeprecated))
		stream := cipher.NewCTR(block, iv)
		stream.XORKeyStream(text, ct)
		if string(text) != x.VerificationTextDeprecated {
			return nil, 0, errors.New("invalid encryption key provided. Please check your encryption key")
		}
		return iv, iterator, nil
	}

	useDeprecated := false
	iv, iterator, err := decryptHeader()
	if err != nil {
		// might have an old audit log
		iv2, iterator2, err := decryptHeaderDeprecated()
		if err != nil {
			return errors.New("invalid encryption key provided. Please check your encryption key")
		}
		// found old audit log
		useDeprecated = true
		iv, iterator = iv2, iterator2
	}

	var count int

	// encrypted writes each have the form below
	// IV generated for each write
	// #################################################################
	// #####   [16]byte IV + [4]byte uint32(len(p)) + [:]byte p    #####
	// #################################################################
	decryptBody := func() {
		for {
			count++
			// if its the end of data. finish decrypting
			if iterator >= stat.Size() {
				break
			}
			n, err := file.ReadAt(iv, iterator)
			if err != nil {
				glog.Infof("received %v while decrypting audit log\n")
				glog.Infof("read %v bytes, expected %v\n", n, len(iv))
			}
			iterator = iterator + 16
			length := make([]byte, 4)
			n, err = file.ReadAt(length, iterator)
			if err != nil {
				glog.Infof("received %v while decrypting audit log\n")
				glog.Infof("read %v bytes, expected %v\n", n, len(iv))
			}
			iterator = iterator + 4

			content := make([]byte, binary.BigEndian.Uint32(length))
			n, err = file.ReadAt(content, iterator)
			if err != nil {
				glog.Infof("received %v while decrypting audit log\n")
				glog.Infof("read %v bytes, expected %v\n", n, len(iv))
			}
			iterator = iterator + int64(binary.BigEndian.Uint32(length))

			stream := cipher.NewCTR(block, iv)
			stream.XORKeyStream(content, content)
			n, err = outfile.Write(content)
			if err != nil {
				glog.Infof("received %v while writing decrypted audit log\n")
				glog.Infof("wrote %v bytes, expected to write %v\n", n, len(iv))
			}
		}
	}

	// encrypted writes in body have the form
	// baseIV is constant, last 4 bytes vary
	// ########################################################
	// #####   [4]byte uint32(len(p)) + [:]byte p         #####
	// ########################################################
	decryptBodyDeprecated := func() {
		for {
			// if its the end of data. finish decrypting
			if iterator >= stat.Size() {
				break
			}
			n, err := file.ReadAt(iv[12:], iterator)
			if err != nil {
				glog.Infof("received %v while decrypting audit log\n")
				glog.Infof("read %v bytes, expected %v\n", n, len(iv))
			}
			iterator = iterator + 4

			content := make([]byte, binary.BigEndian.Uint32(iv[12:]))
			n, err = file.ReadAt(content, iterator)
			if err != nil {
				glog.Infof("received %v while decrypting audit log\n")
				glog.Infof("read %v bytes, expected %v\n", n, len(iv))
			}
			iterator = iterator + int64(binary.BigEndian.Uint32(iv[12:]))
			stream := cipher.NewCTR(block, iv)
			stream.XORKeyStream(content, content)
			n, err = outfile.Write(content)
			if err != nil {
				glog.Infof("received %v while writing decrypted audit log\n")
				glog.Infof("wrote %v bytes, expected to write %v\n", n, len(iv))
			}
		}
	}

	if useDeprecated {
		decryptBodyDeprecated()
	} else {
		decryptBody()
	}

	glog.Infof("Decryption of Audit file %s is Done. Decrypted file is %s",
		decryptCmd.Conf.GetString("in"),
		decryptCmd.Conf.GetString("out"))
	return nil
}
