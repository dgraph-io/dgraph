/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package mcp

import (
	"github.com/golang/glog"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

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
A Dgraph MCP server is a long running process that provides an STDIO
interface for running mcp server.
`,
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
		Annotations: map[string]string{"group": "tool"},
	}
	Mcp.EnvPrefix = "DGRAPH_MCP"
	Mcp.Cmd.SetHelpTemplate(x.NonRootTemplate)

	flag := Mcp.Cmd.Flags()

	flag.StringP("conn-str", "c", "", "Dgraph connection string.")
	flag.Bool("read-only", false, "Run MCP server in read-only mode.")
}

func run() {
	connectionString := Mcp.Conf.GetString("conn-str")
	readOnly := Mcp.Conf.GetBool("read-only")

	s, err := NewMCPServer(connectionString, readOnly)
	if err != nil {
		glog.Errorf("Failed to initialize MCPServer: %v", err)
		return
	}

	// Start the stdio server
	if err := server.ServeStdio(s); err != nil {
		glog.Errorf("Server error: %v", err)
	}
}
