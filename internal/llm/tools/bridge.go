package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/duyseu/llm-core/internal/mcp"
)

// BridgeMCPTools queries all configured MCP servers, lists their tools, and
// returns them bridged into StockMind's internal tool format.
// Each tool's Execute closure routes calls through Manager.CallTool (retry-aware).
func BridgeMCPTools(ctx context.Context, manager *mcp.Manager) ([]*Tool, error) {
	var bridgedTools []*Tool

	for _, serverName := range manager.ConfiguredServers() {
		client, err := manager.GetOrStart(ctx, serverName)
		if err != nil {
			log.Printf("Warning: Failed to connect to MCP server %s: %v. Skipping.", serverName, err)
			continue
		}

		mcpTools, err := client.ListTools(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list tools for %s: %w", serverName, err)
		}

		for _, mt := range mcpTools {
			mcpToolName := mt.Name
			localToolName := fmt.Sprintf("%s_%s", serverName, mcpToolName)

			schemaMap, ok := mt.InputSchema.(map[string]any)
			if !ok {
				schemaBytes, err := json.Marshal(mt.InputSchema)
				if err == nil {
					_ = json.Unmarshal(schemaBytes, &schemaMap)
				}
			}

			// Capture for closure — route through Manager (not a raw *Client pointer).
			targetServer := serverName
			targetTool := mcpToolName

			executeFn := func(ctx context.Context, rawArgs json.RawMessage) (json.RawMessage, error) {
				var args map[string]any
				if len(rawArgs) > 0 && string(rawArgs) != "null" {
					if err := json.Unmarshal(rawArgs, &args); err != nil {
						return nil, fmt.Errorf("failed to unmarshal bridge arguments: %w", err)
					}
				}

				resStr, err := manager.CallTool(ctx, targetServer, targetTool, args)
				if err != nil {
					return nil, fmt.Errorf("mcp bridge error on %s.%s: %w", targetServer, targetTool, err)
				}

				// Return as-is if already valid JSON, otherwise marshal as string.
				if json.Valid([]byte(resStr)) {
					return json.RawMessage(resStr), nil
				}
				fallbackJSON, err := json.Marshal(resStr)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal non-JSON tool result: %w", err)
				}
				return fallbackJSON, nil
			}

			bridgedTools = append(bridgedTools, &Tool{
				Name:        localToolName,
				Description: mt.Description,
				Schema:      schemaMap,
				Execute:     executeFn,
			})
			log.Printf("Dynamic MCP Tool bridged: %s (routes to %s.%s)", localToolName, serverName, mcpToolName)
		}
	}

	return bridgedTools, nil
}
