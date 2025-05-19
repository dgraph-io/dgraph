/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package mcp

import (
	"github.com/golang/glog"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/hypermodeinc/dgraph/v25/mcp"
	"github.com/hypermodeinc/dgraph/v25/x"
)

var (
	Mcp x.SubCommand
)

func init() {
	Mcp.Cmd = &cobra.Command{
		Use:   "mcp",
		Short: "Run Dgraph MCP server",
		Long: `
A Dgraph MCP server is a long running process that provides an STDIO interface for running mcp
`,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Mcp.Conf).Stop()
			run()
		},
		Annotations: map[string]string{"group": "core"},
	}
	Mcp.EnvPrefix = "DGRAPH_MCP"
	Mcp.Cmd.SetHelpTemplate(x.NonRootTemplate)

	flag := Mcp.Cmd.Flags()

	flag.StringP("connection", "c", "dgraph://localhost:9080", "Dgraph connection string.")
	flag.Bool("read-only", false, "Run MCP server in read-only mode.")
}

func run() {
	connectionString := Mcp.Conf.GetString("connection")
	readOnly := Mcp.Conf.GetBool("read_only")

	s, err := mcp.NewMCPServer(connectionString, readOnly)
	if err != nil {
		glog.Errorf("Failed to initialize MCPServer: %v", err)
		return
	}

	// Start the stdio server
	if err := server.ServeStdio(s); err != nil {
		glog.Errorf("Server error: %v", err)
	}
}
