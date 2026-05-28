package mcp

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// ServerConfig stores the parameters required to spawn and configure an external MCP server.
type ServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// Manager orchestrates a thread-safe registry of multiple external MCP clients.
type Manager struct {
	mu      sync.RWMutex
	clients map[string]*Client
	configs map[string]ServerConfig
}

// NewManager creates a new Manager instance from the provided list of server configurations.
func NewManager(configs []ServerConfig) *Manager {
	cfgMap := make(map[string]ServerConfig)
	for _, cfg := range configs {
		cfgMap[cfg.Name] = cfg
	}
	return &Manager{
		clients: make(map[string]*Client),
		configs: cfgMap,
	}
}

// GetOrStart retrieves an existing running MCP client or lazily starts a new session.
func (m *Manager) GetOrStart(ctx context.Context, name string) (*Client, error) {
	m.mu.RLock()
	client, exists := m.clients[name]
	m.mu.RUnlock()
	if exists {
		return client, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock.
	if client, exists = m.clients[name]; exists {
		return client, nil
	}

	config, ok := m.configs[name]
	if !ok {
		return nil, fmt.Errorf("no config found for MCP server: %s", name)
	}

	newClient, err := New(ctx, config.Command, config.Args, config.Env)
	if err != nil {
		return nil, fmt.Errorf("failed to start MCP server %s: %w", name, err)
	}

	m.clients[name] = newClient
	return newClient, nil
}

// CallTool invokes a tool on the named server with automatic retry on dead sessions.
// If the first attempt fails, the client is evicted and a fresh connection is tried once.
func (m *Manager) CallTool(ctx context.Context, server, tool string, args map[string]any) (string, error) {
	log.Printf("[MCP] CallTool: server=%s tool=%s args=%v", server, tool, args)
	client, err := m.GetOrStart(ctx, server)
	if err != nil {
		return "", err
	}

	result, err := client.CallTool(ctx, tool, args)
	if err == nil {
		return result, nil
	}

	// Evict dead client and retry once.
	log.Printf("MCP call %s.%s failed, reconnecting: %v", server, tool, err)
	m.evict(server)

	client, err = m.GetOrStart(ctx, server)
	if err != nil {
		return "", fmt.Errorf("reconnect to %s failed: %w", server, err)
	}
	return client.CallTool(ctx, tool, args)
}

// evict removes and closes a client from the registry.
func (m *Manager) evict(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if client, ok := m.clients[name]; ok {
		_ = client.Close()
		delete(m.clients, name)
	}
}

// ConfiguredServers returns the names of all configured (not necessarily running) servers.
func (m *Manager) ConfiguredServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	return names
}

// CloseAll gracefully terminates all active MCP client sessions.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, client := range m.clients {
		_ = client.Close()
		delete(m.clients, name)
	}
}
