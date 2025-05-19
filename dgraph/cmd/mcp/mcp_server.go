package mcp

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/dgo/v250"
	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/hypermodeinc/dgraph/v25/x"

	"github.com/golang/glog"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

//go:embed prompt.txt
var promptBytes []byte

var dgraphConnection *dgo.Dgraph
var getConnLock sync.Mutex

func getConn(connectionString string) (*dgo.Dgraph, error) {
	getConnLock.Lock()
	defer getConnLock.Unlock()

	if dgraphConnection != nil {
		return dgraphConnection, nil
	}

	conn, err := dgo.Open(connectionString)
	if err != nil {
		for i := range 3 {
			time.Sleep(time.Second * time.Duration(i))
			conn, err = dgo.Open(connectionString)
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, fmt.Errorf("error opening connection with Dgraph Alpha: %v", err)
		}
	}
	dgraphConnection = conn
	return conn, nil
}

// NewMCPServer initializes and returns a new MCPServer instance.
func NewMCPServer(connectionString string, readOnly bool) (*server.MCPServer, error) {
	s := server.NewMCPServer(
		"Dgraph MCP Server",
		x.Version(),
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
		server.WithRecovery(),
	)

	schemaTool := mcp.NewTool("Get-Schema",
		mcp.WithDescription("Get Dgraph DQL Schema from dgraph db"),
		mcp.WithToolAnnotation(mcp.ToolAnnotation{
			ReadOnlyHint:    true,
			DestructiveHint: false,
			IdempotentHint:  true,
			OpenWorldHint:   false,
		}),
	)

	queryTool := mcp.NewTool("Run-Query",
		mcp.WithDescription("Run Dgraph Query on dgraph db"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The query to perform"),
		),
		mcp.WithToolAnnotation(mcp.ToolAnnotation{
			ReadOnlyHint:    true,
			DestructiveHint: false,
			IdempotentHint:  true,
			OpenWorldHint:   false,
		}),
	)

	if !readOnly {
		alterSchemaTool := mcp.NewTool("Alter-Schema",
			mcp.WithDescription("Alter Dgraph DQL Schema in dgraph db"),
			mcp.WithString("schema",
				mcp.Required(),
				mcp.Description("Updated schema to insert inside the db"),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				ReadOnlyHint:    false,
				DestructiveHint: true,
				IdempotentHint:  false,
				OpenWorldHint:   false,
			}),
		)

		s.AddTool(alterSchemaTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			schema, ok := request.Params.Arguments["schema"].(string)
			if !ok {
				return nil, fmt.Errorf("schema must be present")
			}

			// Execute alter operation
			conn, err := getConn(connectionString)
			if err != nil {
				return nil, fmt.Errorf("error opening connection with Dgraph Alpha: %v", err)
			}
			if err = conn.SetSchema(ctx, dgo.RootNamespace, schema); err != nil {
				return nil, fmt.Errorf("schema alteration failed: %v", err)
			}

			return mcp.NewToolResultText("Schema updated successfully"), nil
		})

		mutationTool := mcp.NewTool("Run-Mutation",
			mcp.WithDescription("Run DQL Mutation on dgraph db"),
			mcp.WithString("mutation",
				mcp.Required(),
				mcp.Description("The mutation to perform in json format"),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				ReadOnlyHint:    false,
				DestructiveHint: true,
				IdempotentHint:  false,
				OpenWorldHint:   false,
			}),
		)

		s.AddTool(mutationTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			conn, err := getConn(connectionString)
			if err != nil {
				return nil, err
			}
			txn := conn.NewTxn()
			defer func() {
				err := txn.Discard(ctx)
				if err != nil {
					glog.Errorf("failed to discard transaction: %v", err)
				}
			}()
			mutation, ok := request.Params.Arguments["mutation"].(string)
			if !ok {
				return nil, fmt.Errorf("mutation must present")
			}
			resp, err := txn.Mutate(ctx, &api.Mutation{
				SetJson:   []byte(mutation),
				CommitNow: true,
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(string(resp.GetJson())), nil
		})
	}

	s.AddTool(queryTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		conn, err := getConn(connectionString)
		if err != nil {
			return nil, err
		}
		txn := conn.NewTxn()
		defer func() {
			err := txn.Discard(ctx)
			if err != nil {
				glog.Errorf("failed to discard transaction: %v", err)
			}
		}()
		op := request.Params.Arguments["query"].(string)
		resp, err := txn.Query(ctx, op)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(resp.GetJson())), nil
	})

	s.AddTool(schemaTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		conn, err := getConn(connectionString)
		if err != nil {
			return nil, err
		}
		txn := conn.NewTxn()
		defer func() {
			err := txn.Discard(ctx)
			if err != nil {
				glog.Errorf("failed to discard transaction: %v", err)
			}
		}()
		resp, err := txn.Query(ctx, "schema {}")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(resp.GetJson())), nil
	})

	schemaResource := mcp.NewResource(
		"dgraph://schema",
		"Dgraph Schema",
		mcp.WithResourceDescription("The current Dgraph schema"),
		mcp.WithMIMEType("text/plain"),
	)

	s.AddResource(schemaResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Execute operation
		conn, err := getConn(connectionString)
		if err != nil {
			return nil, err
		}
		resp, err := conn.NewTxn().Query(ctx, "schema {}")
		if err != nil {
			return nil, fmt.Errorf("failed to get schema: %v", err)
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "dgraph://schema",
				MIMEType: "text/plain",
				Text:     string(resp.Json),
			},
		}, nil
	})

	commonQueriesTool := mcp.NewTool("Get-Common-Queries",
		mcp.WithDescription("Get common queries that you can run on the db. If you are seeing issues with your queries, you can check this tool once."),
		mcp.WithToolAnnotation(mcp.ToolAnnotation{
			ReadOnlyHint:    true,
			DestructiveHint: false,
			IdempotentHint:  true,
			OpenWorldHint:   false,
		}),
	)

	s.AddTool(commonQueriesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(`
			{
				"shortest_path_query": "
					{
						q(func: eq(guid, "first guid") { // get first uid in form of a query
							a as uid
						}
						q1(func: eq(guid, "second guid")) { // get second uid in form of a query
							b as uid
						}
						path as shortest(from: uid(a), to: uid(b), numpaths: 5, maxheapsize: 10000) {
							connected_to @facets(weight) // Add @facet() to get path by weight, remove it if all edges are same weight
						}
						path(func: uid(path)) {
							uid
						}
					}
				"`,
		), nil
	})

	commonQueries := mcp.NewResource(
		"dgraph://common-queries",
		"Dgraph common queries",
		mcp.WithResourceDescription("The current Dgraph common queries that you can use to fix your queries"),
		mcp.WithMIMEType("text/plain"),
	)

	s.AddResource(commonQueries, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "dgraph://commmon-queries",
				MIMEType: "text/plain",
				Text: `
				{
					"shortest_path_query": "
						{
							q(func: eq(guid, "first guid")) { // get first uid in form of a query
								a as uid
							}
							q1(func: eq(guid, "second guid")) { // get second uid in form of a query
								b as uid
							}

  							path as shortest(from: uid(a), to: uid(b), numpaths: 5, maxheapsize: 10000) {
  								connected_to @facets(weight) // Add @facet() to get path by weight, remove it if all edges are same weight
 							}

  							path(func: uid(path)) {
   								uid
 							}
						}
					",
				}
				`,
			},
		}, nil
	})

	// Add resource with its handler
	s.AddResource(schemaResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Execute operation
		conn, err := getConn(connectionString)
		if err != nil {
			return nil, err
		}
		resp, err := conn.NewTxn().Query(ctx, "schema {}")
		if err != nil {
			return nil, fmt.Errorf("failed to get schema: %v", err)
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "dgraph://schema",
				MIMEType: "text/plain",
				Text:     string(resp.Json),
			},
		}, nil
	})

	addPrompt(s)

	return s, nil
}

func addPrompt(s *server.MCPServer) {
	prompt := string(promptBytes)
	s.AddPrompt(mcp.NewPrompt("Quick start prompt",
		mcp.WithPromptDescription("A quick Start prompt for new users and llms"),
	), func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return mcp.NewGetPromptResult(
			"A quick start prompt",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleAssistant,
					mcp.NewTextContent(prompt),
				),
			},
		), nil
	})
}
