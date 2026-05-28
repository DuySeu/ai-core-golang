package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client wraps an MCP ClientSession connected to an external server subprocess.
type Client struct {
	session *mcp.ClientSession
}

// New spawns the given command as an MCP server subprocess and establishes an
// MCP session over stdio. The session is fully initialized before New returns.

// command is the executable name (e.g. "uvx"), args are its arguments.
// extraEnv is an optional map of KEY=VALUE pairs appended to the subprocess
// environment (on top of the current process environment).

// The caller is responsible for calling Close when the client is no longer needed.
func New(ctx context.Context, command string, args []string, extraEnv map[string]string) (*Client, error) {
	cmd := exec.Command(command, args...)

	// Build environment: inherit current env, then append extras.
	if len(extraEnv) > 0 {
		env := make([]string, 0, len(extraEnv))
		for k, v := range extraEnv {
			env = append(env, k+"="+v)
		}
		cmd.Env = append(cmd.Environ(), env...)
	}

	transport := &mcp.CommandTransport{Command: cmd}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "stockmind-mcp-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: connect to %q: %w", command, err)
	}

	return &Client{session: session}, nil
}

// CallTool invokes a named tool on the connected MCP server and returns the
// concatenated text content of the result as a string.

// If the tool result carries IsError=true, an error is returned containing
// the tool's error text so the LLM tool loop can handle it gracefully.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return "", fmt.Errorf("mcpclient: call tool %q: %w", name, err)
	}

	// Collect all text content blocks into a single string.
	var sb strings.Builder
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}

	text := sb.String()
	if result.IsError {
		return "", fmt.Errorf("mcpclient: tool %q returned error: %s", name, text)
	}

	return text, nil
}

// ListTools retrieves all available tools from the external MCP server.
func (c *Client) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	result, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: list tools: %w", err)
	}
	return result.Tools, nil
}

// Close gracefully shuts down the MCP session and the underlying subprocess.
func (c *Client) Close() error {
	return c.session.Close()
}
