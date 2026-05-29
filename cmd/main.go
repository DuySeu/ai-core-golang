package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/urfave/cli/v3"

	core "ai-core-golang/internal/llm"
	"ai-core-golang/internal/llm/prompts"
	"ai-core-golang/internal/llm/providers"
	"ai-core-golang/internal/llm/tools"
	"ai-core-golang/internal/mcp"
	"ai-core-golang/internal/rag"
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Printf("Warning: Failed to load .env file: %v\n", err)
	}

	app := cli.Command{
		Name:    "llm-core",
		Usage:   "LLM Core Toolkit CLI",
		Version: "1.0.0",
		Commands: []*cli.Command{
			{
				Name:   "chat",
				Usage:  "Run the console streaming demo",
				Action: runChatCmd,
			},
			{
				Name:   "summarize",
				Usage:  "Run the summarize command",
				Action: runSummarizeCmd,
			},
			{
				Name:   "ingest",
				Usage:  "Ingest knowledge_base/ documents into Qdrant",
				Action: runIngestCmd,
			},
			{
				Name:   "chunking",
				Usage:  "Chunk knowledge_base/ documents and write to chunking.json",
				Action: runChunkingCmd,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "strategy",
						Usage: "Chunking strategy: markdown or semantic",
						Value: "markdown",
					},
				},
			},
			{
				Name:      "search",
				Usage:     "Search the knowledge base directly",
				ArgsUsage: "<query>",
				Action:    runSearchCmd,
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 1. Demo Command
// ──────────────────────────────────────────────────────────────────────────────

func runChatCmd(ctx context.Context, cmd *cli.Command) error {
	// streamPrint writes s in small UTF-8-safe chunks and flushes stdout
	// after each chunk so output appears progressively in the console.
	streamPrint := func(s string) {
		if s == "" {
			return
		}
		rest := s
		for len(rest) > 0 {
			i, n := 0, 0
			for n < 48 && i < len(rest) {
				_, sz := utf8.DecodeRuneInString(rest[i:])
				if sz == 0 {
					break
				}
				i += sz
				n++
			}
			if i == 0 {
				break
			}
			fmt.Print(rest[:i])
			rest = rest[i:]
			_ = os.Stdout.Sync()
		}
	}

	// Initialize MCP client manager and dynamically bridge MCP tools
	var mcpManager *mcp.Manager
	var bridgedMCPTools []*tools.Tool

	var mcpConfigs []mcp.ServerConfig
	if _, err := exec.LookPath("uvx"); err == nil {
		mcpConfigs = append(mcpConfigs, mcp.ServerConfig{
			Name:    "aws-docs",
			Command: "uvx",
			Args: []string{
				"awslabs.aws-documentation-mcp-server@latest",
			},
			Env: map[string]string{
				"FASTMCP_LOG_LEVEL":           "ERROR",
				"AWS_DOCUMENTATION_PARTITION": "aws",
			},
		})
	} else {
		log.Println("Warning: uvx not found — external MCP clients like AWS documentation disabled")
	}

	if len(mcpConfigs) > 0 {
		mcpManager = mcp.NewManager(mcpConfigs)

		bridgeCtx, bridgeCancel := context.WithTimeout(ctx, 30*time.Second)
		defer bridgeCancel()

		var err error
		bridgedMCPTools, err = tools.BridgeMCPTools(bridgeCtx, mcpManager)
		if err != nil {
			log.Printf("Warning: failed to bridge MCP tools: %v", err)
		} else {
			log.Printf("Successfully bridged %d dynamic MCP tools", len(bridgedMCPTools))
		}
	}

	// Initialize tools and LLM service
	cfg := providers.LoadServerConfig()
	embedder := rag.NewEmbedderWithClient(providers.SharedHTTPClient)
	qdrantClient := rag.NewQdrantClientWithHTTP(rag.QdrantConfig{
		Host:       cfg.Qdrant.Host,
		Port:       cfg.Qdrant.Port,
		APIKey:     cfg.Qdrant.APIKey,
		Collection: cfg.Qdrant.Collection,
		UseHTTPS:   cfg.Qdrant.UseHTTPS,
	}, providers.SharedHTTPClient)
	ragTool := tools.NewSearchKnowledgeBaseTool(embedder, qdrantClient)

	toolDefs := tools.RegisterTools(ragTool)
	if len(bridgedMCPTools) > 0 {
		toolDefs = append(toolDefs, bridgedMCPTools...)
	}
	toolMgr := tools.NewManager(toolDefs)
	svc, err := core.NewLLMService(ctx, cfg.LLM, toolMgr)
	if err != nil {
		return fmt.Errorf("failed to initialize LLM service: %w", err)
	}

	// ──────── System prompt ────────
	loader := prompts.NewPromptLoader()
	systemPrompt, err := loader.GetSystemPrompt(prompts.SystemParams{
		Date:     time.Now().Format("2006-01-02"),
		Summary:  "",
		KeyFacts: "",
	})
	if err != nil {
		return fmt.Errorf("failed to get system prompt: %w", err)
	}

	// Run Chat
	sessionID := uuid.New().String()
	fmt.Printf("--- Starting Session: %s ---\n", sessionID)

	stream, err := svc.LLMChat(ctx, []providers.Message{
		{Role: "user", Content: "Tìm tài liệu AWS về cách cấu hình Lambda function URL với IAM authentication, và cho tôi biết các bước chính để setup."},
	}, core.LLMOptions{SystemPrompt: systemPrompt})
	if err != nil {
		return err
	}

	for event := range stream {
		switch event.Type {
		case providers.EventText:
			fmt.Print(event.Content)
		case providers.EventToolCall:
			tc := event.Data.(providers.Tool)
			fmt.Printf("\n[🔧 Calling Tool: %s]\n", tc.Name)
			fmt.Print("    args: ")
			streamPrint(tc.Arguments)
			fmt.Println()
		case providers.EventToolResult:
			tr := event.Data.(providers.Tool)
			if tr.IsError == "true" {
				fmt.Print("\n[❌ Tool error] ")
				streamPrint(tr.Output)
				fmt.Println()
				continue
			}
			fmt.Print("\n[✅ Tool Result] ")
			streamPrint(tr.Output)
			fmt.Println()
		case providers.EventError:
			fmt.Printf("\n[❌ Error: %v]\n", event.Data)
		case providers.EventDone:
			fmt.Println("\n--- Done ---")
		}
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// 2. Summarize Command
// ──────────────────────────────────────────────────────────────────────────────

func runSummarizeCmd(ctx context.Context, cmd *cli.Command) error {
	// Build LLM Core Service dependencies
	cfg := providers.LoadServerConfig()

	// Initialize tools and LLM service
	toolDefs := tools.RegisterTools()
	toolMgr := tools.NewManager(toolDefs)

	svc, err := core.NewLLMService(ctx, cfg.LLM, toolMgr)
	if err != nil {
		return fmt.Errorf("failed to initialize LLM service: %w", err)
	}

	// ──────── System prompt ────────
	loader := prompts.NewPromptLoader()
	_, err = loader.GetSystemPrompt(prompts.SystemParams{
		Date:     time.Now().Format("2006-01-02"),
		Summary:  "",
		KeyFacts: "",
	})
	if err != nil {
		return fmt.Errorf("failed to get system prompt: %w", err)
	}

	// Run Summarize
	summarizeResult, err := svc.Summarize([]providers.Message{
		{Role: "user", Content: "Tôi đang xây dựng một hệ thống microservices trên AWS. Hiện tại có 3 services: user-service, order-service, và payment-service."},
		{Role: "assistant", Content: "Hệ thống microservices của bạn gồm 3 services chính. Bạn cần hỗ trợ gì về kiến trúc hoặc triển khai?"},
		{Role: "user", Content: "Tôi muốn dùng Lambda function URL thay vì API Gateway để giảm chi phí. Mỗi service sẽ là một Lambda function."},
		{Role: "assistant", Content: "Lambda function URL là lựa chọn tốt để giảm chi phí. Bạn sẽ không phải trả phí API Gateway, chỉ trả phí Lambda invocation. Tuy nhiên cần lưu ý về authentication và rate limiting."},
		{Role: "user", Content: "Về authentication, tôi muốn dùng IAM auth cho internal service-to-service communication và không auth cho public endpoints."},
		{Role: "assistant", Content: "Đó là pattern phổ biến. IAM auth cho internal calls đảm bảo chỉ các services trong account mới gọi được nhau. Public endpoints dùng AuthType NONE nhưng bạn nên implement JWT validation trong code."},
		{Role: "user", Content: "Payment service cần kết nối với Stripe webhook. Stripe sẽ gọi vào payment-service Lambda URL. Làm sao verify webhook signature?"},
		{Role: "assistant", Content: "Với Stripe webhooks, bạn cần: 1) Set AuthType NONE cho webhook endpoint, 2) Verify Stripe signature trong code bằng webhook secret, 3) Validate event timestamp để chống replay attacks."},
		{Role: "user", Content: "Database thì sao? Mỗi service có nên có database riêng không?"},
		{Role: "assistant", Content: "Theo database-per-service pattern, mỗi service nên có DB riêng để đảm bảo loose coupling. User-service dùng DynamoDB cho user profiles, order-service dùng PostgreSQL (RDS) cho transactions, payment-service dùng DynamoDB cho payment records."},
		{Role: "user", Content: "Nếu order-service cần thông tin user thì gọi API user-service hay query trực tiếp DB?"},
		{Role: "assistant", Content: "Luôn gọi qua API, không query trực tiếp DB của service khác. Dùng IAM-authenticated Lambda URL call. Có thể cache response với ElastiCache nếu cần performance."},
	}, core.SummarizeResult{})
	if err != nil {
		return fmt.Errorf("failed to summarize: %w", err)
	}
	out, _ := json.MarshalIndent(summarizeResult, "", "  ")
	fmt.Println(string(out))
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// 3. Ingest Command
// ──────────────────────────────────────────────────────────────────────────────

func runIngestCmd(ctx context.Context, cmd *cli.Command) error {
	cfg := providers.LoadServerConfig()
	embedder := rag.NewEmbedderWithClient(providers.SharedHTTPClient)
	qdrantClient := rag.NewQdrantClientWithHTTP(rag.QdrantConfig{
		Host:       cfg.Qdrant.Host,
		Port:       cfg.Qdrant.Port,
		APIKey:     cfg.Qdrant.APIKey,
		Collection: cfg.Qdrant.Collection,
		UseHTTPS:   cfg.Qdrant.UseHTTPS,
	}, providers.SharedHTTPClient)

	files, chunks, err := rag.Ingest(ctx, embedder, qdrantClient)
	if err != nil {
		return fmt.Errorf("ingest failed: %w", err)
	}
	fmt.Printf("---\nIngested %d files, %d vectors stored in Qdrant.\n", files, chunks)
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// 4. Chunking Command
// ──────────────────────────────────────────────────────────────────────────────

func runChunkingCmd(ctx context.Context, cmd *cli.Command) error {
	strategyName := cmd.String("strategy")
	embedder := rag.NewEmbedder()
	chunker, err := rag.GetChunker(strategyName, embedder)
	if err != nil {
		return err
	}

	chunks, err := rag.ChunkKnowledgeBase(ctx, chunker)
	if err != nil {
		return fmt.Errorf("chunking failed: %w", err)
	}
	out, err := json.MarshalIndent(chunks, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile("chunking.json", out, 0644); err != nil {
		return fmt.Errorf("write chunking.json: %w", err)
	}
	fmt.Printf("Strategy: %s | Wrote %d chunks to chunking.json\n", strategyName, len(chunks))
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// 5. Search Command
// ──────────────────────────────────────────────────────────────────────────────

func runSearchCmd(ctx context.Context, cmd *cli.Command) error {
	query := cmd.Args().First()
	if query == "" {
		return fmt.Errorf("usage: llm-core search <query>")
	}

	cfg := providers.LoadServerConfig()
	embedder := rag.NewEmbedderWithClient(providers.SharedHTTPClient)
	qdrantClient := rag.NewQdrantClientWithHTTP(rag.QdrantConfig{
		Host:       cfg.Qdrant.Host,
		Port:       cfg.Qdrant.Port,
		APIKey:     cfg.Qdrant.APIKey,
		Collection: cfg.Qdrant.Collection,
		UseHTTPS:   cfg.Qdrant.UseHTTPS,
	}, providers.SharedHTTPClient)

	vecs, err := embedder.Embed(ctx, []string{query})
	if err != nil {
		return fmt.Errorf("embed query: %w", err)
	}

	results, err := qdrantClient.Search(ctx, vecs[0], 5, 0.7)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	if len(results) == 0 {
		fmt.Printf("No relevant documents found for: %q\n", query)
		return nil
	}

	for _, r := range results {
		fmt.Printf("[%.2f] %s > %s\n", r.Score, r.Payload["source_file"], r.Payload["heading_path"])
		// Print first 200 chars of content as preview
		content := r.Payload["content"]
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Printf("  %s\n\n", content)
	}
	return nil
}
