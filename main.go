// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/go-github/v60/github"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"

	benchmarkcmd "github.com/doc-scout/mcp-server/benchmark/cmd"
	"github.com/doc-scout/mcp-server/embeddings"
	"github.com/doc-scout/mcp-server/health"
	"github.com/doc-scout/mcp-server/indexer"
	"github.com/doc-scout/mcp-server/memory"
	"github.com/doc-scout/mcp-server/scanner"
	"github.com/doc-scout/mcp-server/scanner/parser"
	"github.com/doc-scout/mcp-server/tools"
	"github.com/doc-scout/mcp-server/webhook"
)

// serverHealthProvider implements health.StatusProvider using the live scanner and graph.

type serverHealthProvider struct {
	sc *scanner.Scanner

	graph tools.GraphStore

	startedAt time.Time
}

func (p *serverHealthProvider) HealthStatus() health.Status {

	_, _, repoCount := p.sc.Status()

	status := "starting"

	if repoCount > 0 {

		status = "ok"

	}

	var entities int64

	if p.graph != nil {

		entities, _ = p.graph.EntityCount()

	}

	return health.Status{

		Status: status,

		StartedAt: p.startedAt,

		RepoCount: repoCount,

		Entities: entities,
	}

}

const (
	serverName = "DocScout-MCP"

	defaultScanInterval = 30 * time.Minute

	defaultMaxContent = 200 * 1024 // 200 KB

)

var serverVersion = "dev" // set at build time via -ldflags "-X main.serverVersion=<tag>"

func parseScanInterval(raw string) time.Duration {

	if raw == "" {

		return defaultScanInterval

	}

	if d, err := time.ParseDuration(raw); err == nil && d > 0 {

		return d

	}

	if n, err := strconv.Atoi(raw); err == nil && n > 0 {

		return time.Duration(n) * time.Minute

	}

	slog.Warn("Invalid SCAN_INTERVAL, using default", "raw", raw, "default", defaultScanInterval)

	return defaultScanInterval

}

func parseCSVEnv(raw string) []string {

	raw = strings.TrimSpace(raw)

	if raw == "" {

		return nil

	}

	var result []string

	for p := range strings.SplitSeq(raw, ",") {

		if p = strings.TrimSpace(p); p != "" {

			result = append(result, p)

		}

	}

	if len(result) == 0 {

		return nil

	}

	return result

}

// isInMemoryDB returns true when the DB URL refers to an in-memory SQLite instance.

func isInMemoryDB(dbURL string) bool {

	return dbURL == "" || strings.Contains(dbURL, ":memory:")

}

func main() {

	if len(os.Args) > 1 && os.Args[1] == "--benchmark" {
		os.Exit(benchmarkcmd.Run(os.Args[2:]))
	}

	// Configure slog to write to stderr to prevent MCP stdio corruption

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	slog.SetDefault(logger)

	token := os.Getenv("GITHUB_TOKEN")

	if token == "" {

		slog.Error("GITHUB_TOKEN environment variable is required")

		os.Exit(1)

	}

	org := os.Getenv("GITHUB_ORG")

	if org == "" {

		slog.Error("GITHUB_ORG environment variable is required")

		os.Exit(1)

	}

	scanInterval := parseScanInterval(os.Getenv("SCAN_INTERVAL"))

	targetFiles := parseCSVEnv(os.Getenv("SCAN_FILES"))

	scanDirs := parseCSVEnv(os.Getenv("SCAN_DIRS"))

	infraDirs := parseCSVEnv(os.Getenv("SCAN_INFRA_DIRS"))

	extraRepos := parseCSVEnv(os.Getenv("EXTRA_REPOS"))

	repoTopics := parseCSVEnv(os.Getenv("REPO_TOPICS"))

	var repoRegex *regexp.Regexp

	if rx := os.Getenv("REPO_REGEX"); rx != "" {

		compiled, err := regexp.Compile(rx)

		if err != nil {

			slog.Error("Invalid REPO_REGEX", "regex", rx, "error", err)

			os.Exit(1)

		}

		repoRegex = compiled

	}

	httpAddr := os.Getenv("HTTP_ADDR")

	dbURL := os.Getenv("DATABASE_URL")

	if dbURL == "" {

		dbURL = os.Getenv("MEMORY_FILE_PATH") // backward compatibility

	}

	scanContent := strings.EqualFold(os.Getenv("SCAN_CONTENT"), "true")

	graphReadOnly := strings.EqualFold(os.Getenv("GRAPH_READ_ONLY"), "true")

	if graphReadOnly {

		slog.Info("Graph read-only mode enabled: mutation tools (create_entities, create_relations, add_observations, delete_*) will not be registered")

	}

	maxContentSize := defaultMaxContent

	if raw := os.Getenv("MAX_CONTENT_SIZE"); raw != "" {

		if n, err := strconv.Atoi(raw); err == nil && n > 0 {

			maxContentSize = n

		} else {

			slog.Warn("Invalid MAX_CONTENT_SIZE, using default", "raw", raw, "default", defaultMaxContent)

		}

	}

	// Disable content caching when using in-memory SQLite — data would be lost on restart.

	if scanContent && isInMemoryDB(dbURL) {

		slog.Error("SCAN_CONTENT=true requires a persistent DATABASE_URL. Content caching has been disabled. " +

			"Set DATABASE_URL to a SQLite file path (e.g. sqlite:///data/docs.db) or a PostgreSQL URL to enable full-text search.")

		scanContent = false

	}

	// --- Context & Graceful Shutdown ---

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	defer cancel()

	// --- GitHub client ---

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})

	httpClient := oauth2.NewClient(ctx, ts)

	ghClient := github.NewClient(httpClient)

	// --- Parser Registry ---

	parser.Register(parser.GoModParser())

	parser.Register(parser.PackageJSONParser())

	parser.Register(parser.PomParser())

	parser.Register(parser.CodeownersParser())

	parser.Register(parser.CatalogParser())

	// Integration topology parsers (#15)

	parser.Register(parser.AsyncAPIParser())

	parser.Register(parser.SpringKafkaParser())

	parser.Register(parser.OpenAPIParser())

	parser.Register(parser.ProtoParser())

	parser.Register(parser.K8sServiceParser())

	// --- Scanner ---

	sc := scanner.New(ghClient, org, scanInterval, targetFiles, scanDirs, infraDirs, extraRepos, repoTopics, repoRegex, parser.Default)

	// Warn operators that active repo filters will cause excluded repos' entities to be archived.

	if repoRegex != nil || len(repoTopics) > 0 {

		slog.Warn("Repository filters are active. Entities from repos excluded by these filters will be marked _status:archived on the next scan.",

			"REPO_REGEX", os.Getenv("REPO_REGEX"),

			"REPO_TOPICS", os.Getenv("REPO_TOPICS"))

	}

	// --- Database ---

	db, err := memory.OpenDB(dbURL)

	if err != nil {

		slog.Error("Failed to open database", "error", err)

		os.Exit(1)

	}

	// --- MCP Server ---

	mcpServer := mcp.NewServer(&mcp.Implementation{

		Name: serverName,

		Version: serverVersion,
	}, nil)

	// --- Memory / Knowledge Graph ---

	memorySrv := memory.NewMemoryService(db)

	// Wrap with audit logger — logs every graph mutation to slog (stderr).

	auditedGraph := tools.NewGraphAuditLogger(memorySrv)

	// --- Content Cache ---

	var contentCache *memory.ContentCache

	if scanContent {

		contentCache = memory.NewContentCache(db, true, maxContentSize)

		slog.Info("Content caching enabled", "maxFileSize", maxContentSize)

	}

	// --- Semantic Search Plus ---

	embCfg := embeddings.ConfigFromEnv()

	embProvider := embeddings.NewProvider(embCfg)

	var semanticSrv tools.SemanticSearch

	if embProvider != nil {

		embStore, err := embeddings.NewVectorStore(db)

		if err != nil {

			slog.Error("Failed to create vector store", "error", err)

			os.Exit(1)

		}

		var docSrc embeddings.DocStore

		if contentCache != nil {

			docSrc = contentCache

		}

		embIndexer := embeddings.NewIndexer(embProvider, embStore, docSrc, memorySrv)

		semanticSrv = embeddings.NewSemanticSearcher(embProvider, embStore, embIndexer, docSrc, memorySrv)

		slog.Info("[embeddings] Semantic search enabled", "provider", embProvider.ModelKey())

	} else {

		slog.Info("[embeddings] Semantic search disabled (no DOCSCOUT_EMBED_OPENAI_KEY or DOCSCOUT_EMBED_OLLAMA_URL)")

	}

	// --- Tool Metrics ---

	toolMetrics := tools.NewToolMetrics()

	docMetrics := tools.NewDocMetrics()

	// --- Auto-Indexer ---

	ai := indexer.New(sc, auditedGraph, contentCache, parser.Default)

	sc.SetOnScanComplete(func(repos []scanner.RepoInfo) {

		start := time.Now()

		slog.Info("[indexer] Auto-indexing started", "repos", len(repos))

		ai.Run(context.Background(), repos)

		slog.Info("[indexer] Auto-indexing complete", "duration", time.Since(start).String())

		// Map concrete pointers to interface accurately to avoid typed-nils

		var searcher tools.ContentSearcher

		if contentCache != nil {

			searcher = contentCache

		}

		// Re-register tools to implicitly trigger the MCP tools/list_changed notification

		tools.Register(mcpServer, sc, auditedGraph, searcher, semanticSrv, toolMetrics, docMetrics, graphReadOnly)

		slog.Info("Triggered tools/list_changed notification")

		if semanticSrv != nil {

			for _, repo := range repos {

				go semanticSrv.IndexDocs(context.Background(), repo.FullName)

			}

		}

	})

	// --- Register MCP Tools ---

	var searcher tools.ContentSearcher

	if contentCache != nil {

		searcher = contentCache

	}

	tools.Register(mcpServer, sc, auditedGraph, searcher, semanticSrv, toolMetrics, docMetrics, graphReadOnly)

	// --- Start scanner (initial + periodic) ---

	sc.Start(ctx)

	slog.Info("Server starting", "name", serverName, "version", serverVersion, "org", org, "scan_interval", scanInterval)

	// --- Transport ---

	if httpAddr != "" {

		slog.Info("Listening on Streamable HTTP", "addr", httpAddr)

		mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {

			return mcpServer

		}, nil)

		healthProvider := &serverHealthProvider{

			sc: sc,

			graph: auditedGraph,

			startedAt: time.Now(),
		}

		mux := http.NewServeMux()

		mux.Handle("/", mcpHandler)

		// /healthz is intentionally unauthenticated — load balancers and K8s probes

		// need to reach it without a bearer token.

		mux.HandleFunc("/healthz", health.Handler(healthProvider))

		mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {

			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

			snapshot := toolMetrics.Snapshot()

			fmt.Fprintf(w, "# HELP docscout_tool_calls_total Total number of MCP tool calls since server start.\n")

			fmt.Fprintf(w, "# TYPE docscout_tool_calls_total counter\n")

			for tool, count := range snapshot {

				fmt.Fprintf(w, "docscout_tool_calls_total{tool=%q} %d\n", tool, count)

			}

			fmt.Fprintf(w, "# HELP docscout_document_accesses_total Total number of times a specific document was fetched or returned in content search results since server start.\n")

			fmt.Fprintf(w, "# TYPE docscout_document_accesses_total counter\n")

			for _, d := range docMetrics.TopN(0) {

				fmt.Fprintf(w, "docscout_document_accesses_total{repo=%q,path=%q} %d\n", d.Repo, d.Path, d.Count)

			}

		})

		// GitHub Webhooks — optional, enabled only when GITHUB_WEBHOOK_SECRET is set.

		// The endpoint uses its own HMAC-SHA256 validation and bypasses bearer token auth.

		if webhookSecret := os.Getenv("GITHUB_WEBHOOK_SECRET"); webhookSecret != "" {

			mux.Handle("/webhook", webhook.Handler(ctx, []byte(webhookSecret), sc))

			slog.Info("GitHub webhook endpoint enabled", "path", "/webhook")

		}

		// Bearer Token Auth Middleware — uses constant-time comparison to prevent timing attacks.

		// /webhook and /healthz are explicitly excluded: they carry their own auth or are

		// intentionally public for infrastructure probes.

		authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			if r.URL.Path == "/webhook" || r.URL.Path == "/healthz" {

				mux.ServeHTTP(w, r)

				return

			}

			expectedToken := os.Getenv("MCP_HTTP_BEARER_TOKEN")

			if expectedToken != "" {

				provided := r.Header.Get("Authorization")

				expected := "Bearer " + expectedToken

				if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {

					http.Error(w, "Unauthorized", http.StatusUnauthorized)

					return

				}

			}

			mux.ServeHTTP(w, r)

		})

		srv := &http.Server{

			Addr: httpAddr,

			Handler: authHandler,

			ReadTimeout: 30 * time.Second,

			WriteTimeout: 60 * time.Second,

			IdleTimeout: 120 * time.Second,
		}

		go func() {

			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {

				slog.Error("HTTP server error", "error", err)

				os.Exit(1)

			}

		}()

		<-ctx.Done()

		slog.Info("Shutting down HTTP server...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)

		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {

			slog.Error("HTTP server shutdown error", "error", err)

		}

	} else {

		slog.Info("Listening on stdio...")

		go func() {

			<-ctx.Done()

			slog.Info("Received shutdown signal, exiting...")

			os.Exit(0)

		}()

		if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil {

			if err != context.Canceled {

				slog.Error("Server error", "error", err)

				os.Exit(1)

			}

		}

	}

}
