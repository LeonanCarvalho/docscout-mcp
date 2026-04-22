// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package scanner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v60/github"

	"github.com/doc-scout/mcp-server/scanner/parser"
)

// DefaultTargetFiles is kept for backward compatibility but is no longer used by New().

// Target files are now computed from the ParserRegistry at construction time.

// Deprecated: pass a ParserRegistry to New() instead.

var DefaultTargetFiles = []string{}

// staticTargetFiles are files indexed as documentation or infra assets that have no

// registered FileParser (they don't produce graph entities).

var staticTargetFiles = []string{

	"README.md",

	"mkdocs.yml",

	"openapi.yaml",

	"swagger.json",

	"SKILLS.md",

	"AGENTS.md",

	// Infrastructure / tooling files at repo root.

	"Dockerfile",

	"docker-compose.yml",

	"docker-compose.yaml",

	"Makefile",

	".mise.toml",

	"mise.toml",
}

// DefaultScanDirs defines the default directories to scan recursively for .md files.

var DefaultScanDirs = []string{

	"docs",

	".agents",
}

// DefaultInfraDirs defines the default directories to scan recursively for

// infrastructure and deployment files (.yaml, .yml, .tf, .hcl, .toml).

var DefaultInfraDirs = []string{

	"deploy",

	"infra",

	".github/workflows",
}

// infraExtensions is the set of file extensions indexed when scanning infra directories.

var infraExtensions = map[string]bool{

	".yaml": true,

	".yml": true,

	".tf": true,

	".hcl": true,

	".toml": true,

	".proto": true,
}

// FileEntry represents an indexed documentation file.

type FileEntry struct {
	RepoName string `json:"repo_name"`

	Path string `json:"path"`

	SHA string `json:"sha"`

	Type string `json:"type"` // e.g. "catalog-info", "mkdocs", "openapi", "swagger", "readme", "docs"

}

// RepoInfo holds metadata about a repository that contains documentation.

type RepoInfo struct {
	Name string `json:"name"`

	FullName string `json:"full_name"`

	Description string `json:"description"`

	HTMLURL string `json:"html_url"`

	Files []FileEntry `json:"files"`
}

// Scanner manages GitHub org scanning and caching.

type Scanner struct {
	client *github.Client

	org string

	scanInterval time.Duration

	targetFiles []string // files to look for at repo root

	scanDirs []string // directories to scan recursively for .md files

	infraDirs []string // directories to scan recursively for infra files (.yaml, .tf, .hcl, .toml)

	extraRepos []string // extra explicit repos formatted as "owner/repo"

	repoTopics []string // filter org repos by topics

	repoRegex *regexp.Regexp // filter org repos by name using regex

	registry *parser.ParserRegistry

	mu sync.RWMutex

	repos map[string]*RepoInfo // keyed by repo name

	scanning bool

	lastScanAt time.Time

	onScanComplete func([]RepoInfo) // called after each full scan completes

	triggerCh chan struct{} // signals an on-demand full scan; buffered(1) so TriggerScan never blocks

}

// New creates a new Scanner instance. registry must be non-nil.

// targetFiles are computed from the registry (parser filenames) plus staticTargetFiles.

// Pass non-nil targetFilesOverride to add extra root-level files beyond the defaults.

func New(client *github.Client, org string, scanInterval time.Duration, targetFilesOverride, scanDirs, infraDirs, extraRepos, repoTopics []string, repoRegex *regexp.Regexp, registry *parser.ParserRegistry) *Scanner {

	// Compute default target files from registry + static set.

	seen := make(map[string]bool)

	var targetFiles []string

	for _, fn := range registry.TargetFilenames() {

		if !seen[fn] {

			seen[fn] = true

			targetFiles = append(targetFiles, fn)

		}

	}

	for _, fn := range staticTargetFiles {

		if !seen[fn] {

			seen[fn] = true

			targetFiles = append(targetFiles, fn)

		}

	}

	// Apply user override if supplied.

	if len(targetFilesOverride) > 0 {

		for _, fn := range targetFilesOverride {

			if !seen[fn] {

				seen[fn] = true

				targetFiles = append(targetFiles, fn)

			}

		}

	}

	if len(scanDirs) == 0 {

		scanDirs = DefaultScanDirs

	}

	if len(infraDirs) == 0 {

		infraDirs = DefaultInfraDirs

	}

	return &Scanner{

		client: client,

		org: org,

		scanInterval: scanInterval,

		targetFiles: targetFiles,

		scanDirs: scanDirs,

		infraDirs: infraDirs,

		extraRepos: extraRepos,

		repoTopics: repoTopics,

		repoRegex: repoRegex,

		registry: registry,

		repos: make(map[string]*RepoInfo),

		triggerCh: make(chan struct{}, 1),
	}

}

// SetOnScanComplete registers a callback invoked after each full scan with the current repo list.

// The callback runs synchronously in the scan goroutine.

func (s *Scanner) SetOnScanComplete(fn func([]RepoInfo)) {

	s.mu.Lock()

	defer s.mu.Unlock()

	s.onScanComplete = fn

}

// Start begins the initial scan and schedules periodic re-scans.

func (s *Scanner) Start(ctx context.Context) {

	// Initial scan in a goroutine so it doesn't block server startup.

	go func() {

		slog.Info("[scanner] Starting initial org scan")

		s.scanOrg(ctx)

		slog.Info("[scanner] Initial scan complete", "repos", len(s.repos))

	}()

	// Periodic re-scan and on-demand trigger loop.

	go func() {

		var ticker *time.Ticker

		var tickerC <-chan time.Time

		if s.scanInterval > 0 {

			ticker = time.NewTicker(s.scanInterval)

			tickerC = ticker.C

			defer ticker.Stop()

		}

		for {

			select {

			case <-ctx.Done():

				return

			case <-tickerC:

				slog.Info("[scanner] Starting periodic re-scan")

				s.scanOrg(ctx)

				slog.Info("[scanner] Re-scan complete", "repos", len(s.repos))

			case <-s.triggerCh:

				slog.Info("[scanner] Starting on-demand scan (triggered)")

				s.scanOrg(ctx)

				slog.Info("[scanner] On-demand scan complete", "repos", len(s.repos))

			}

		}

	}()

}

// TriggerScan queues an immediate full organisation scan.

// Returns true if the scan was queued, false if one was already queued (the queue

// holds at most one pending request, so duplicate triggers are coalesced).

func (s *Scanner) TriggerScan() bool {

	select {

	case s.triggerCh <- struct{}{}:

		return true

	default:

		return false

	}

}

// scanOrg lists all repos and scans each for target documentation files.

func (s *Scanner) scanOrg(ctx context.Context) {

	s.mu.Lock()

	s.scanning = true

	s.mu.Unlock()

	defer func() {

		s.mu.Lock()

		s.scanning = false

		s.lastScanAt = time.Now()

		s.mu.Unlock()

	}()

	allRepos, err := s.listAllRepos(ctx)

	if err != nil {

		slog.Error("[scanner] Error listing repos", "error", err)

		return

	}

	slog.Info("[scanner] Found total repos", "count", len(allRepos), "org", s.org)

	// Start concurrent scanning with semaphore

	const maxConcurrency = 5

	sem := make(chan struct{}, maxConcurrency)

	var wg sync.WaitGroup

	newRepos := make(map[string]*RepoInfo)

	var mu sync.Mutex

	for _, repo := range allRepos {

		repoName := repo.GetName()

		sem <- struct{}{} // acquire

		wg.Go(func() {

			defer func() { <-sem }() // release

			repoOwner := repo.GetOwner().GetLogin()

			// Per-repo timeout prevents a single slow GitHub response from stalling the scan.

			repoCtx, repoCancel := context.WithTimeout(ctx, 30*time.Second)

			defer repoCancel()

			files := s.scanRepo(repoCtx, repoOwner, repoName)

			if len(files) > 0 {

				info := &RepoInfo{

					Name: fmt.Sprintf("%s/%s", repoOwner, repoName),

					FullName: repo.GetFullName(),

					Description: repo.GetDescription(),

					HTMLURL: repo.GetHTMLURL(),

					Files: files,
				}

				mu.Lock()

				newRepos[info.Name] = info

				mu.Unlock()

			}

		})

	}

	wg.Wait()

	// Swap entire cache atomically.

	s.mu.Lock()

	s.repos = newRepos

	onComplete := s.onScanComplete

	s.mu.Unlock()

	// Invoke callback outside the lock to avoid deadlock if callback calls ListRepos.

	if onComplete != nil {

		onComplete(s.ListRepos())

	}

}

// listAllRepos paginates through all repos for the configured owner.

// It applies REPO_TOPICS and REPO_REGEX filters, and also fetches EXTRA_REPOS.

func (s *Scanner) listAllRepos(ctx context.Context) ([]*github.Repository, error) {

	repos, err := s.listByOrg(ctx)

	if err != nil {

		// Check if the error is a 404 (owner is a user, not an org).

		if ghErr, ok := errors.AsType[*github.ErrorResponse](err); ok && ghErr.Response != nil && ghErr.Response.StatusCode == 404 {

			slog.Info("[scanner] Not an org, trying as user account", "owner", s.org)

			repos, err = s.listByUser(ctx)

		}

		if err != nil {

			return nil, err

		}

	}

	var filtered []*github.Repository

	for _, r := range repos {

		// Filter by Regex

		if s.repoRegex != nil && !s.repoRegex.MatchString(r.GetName()) {

			continue

		}

		// Filter by Topics

		if len(s.repoTopics) > 0 {

			matchTopic := slices.ContainsFunc(r.Topics, func(t string) bool {

				return slices.ContainsFunc(s.repoTopics, func(req string) bool {

					return strings.EqualFold(t, req)

				})

			})

			if !matchTopic {

				continue

			}

		}

		filtered = append(filtered, r)

	}

	// Append Extra Repos

	for _, er := range s.extraRepos {

		parts := strings.SplitN(er, "/", 2)

		if len(parts) != 2 {

			slog.Warn("[scanner] Invalid EXTRA_REPOS format, skipping", "entry", er)

			continue

		}

		var r *github.Repository

		if err := retryGitHub(ctx, func() error {

			var e error

			r, _, e = s.client.Repositories.Get(ctx, parts[0], parts[1])

			return e

		}); err != nil {

			slog.Warn("[scanner] Error fetching extra repo", "repo", er, "error", err)

			continue

		}

		filtered = append(filtered, r)

	}

	return filtered, nil

}

func (s *Scanner) listByOrg(ctx context.Context) ([]*github.Repository, error) {

	var all []*github.Repository

	opts := &github.RepositoryListByOrgOptions{

		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {

		var repos []*github.Repository

		var resp *github.Response

		err := retryGitHub(ctx, func() error {

			var e error

			repos, resp, e = s.client.Repositories.ListByOrg(ctx, s.org, opts)

			return e

		})

		if err != nil {

			return nil, err

		}

		all = append(all, repos...)

		if resp.NextPage == 0 {

			break

		}

		opts.Page = resp.NextPage

	}

	return all, nil

}

func (s *Scanner) listByUser(ctx context.Context) ([]*github.Repository, error) {

	var all []*github.Repository

	opts := &github.RepositoryListByUserOptions{

		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {

		var repos []*github.Repository

		var resp *github.Response

		err := retryGitHub(ctx, func() error {

			var e error

			repos, resp, e = s.client.Repositories.ListByUser(ctx, s.org, opts)

			return e

		})

		if err != nil {

			return nil, fmt.Errorf("listing user repos page %d: %w", opts.Page, err)

		}

		all = append(all, repos...)

		if resp.NextPage == 0 {

			break

		}

		opts.Page = resp.NextPage

	}

	return all, nil

}

// TriggerRepoScan performs an immediate targeted scan of a single repository and

// invokes the onScanComplete callback with the full updated repo list.

// It is intended for use by the webhook handler to react to GitHub push/create/delete events.

// The call blocks until the scan and callback complete.

func (s *Scanner) TriggerRepoScan(ctx context.Context, owner, repoName string) {

	repoCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

	defer cancel()

	// Fetch current repo metadata (description, URL, visibility) from GitHub.

	var ghRepo *github.Repository

	if err := retryGitHub(repoCtx, func() error {

		var e error

		ghRepo, _, e = s.client.Repositories.Get(repoCtx, owner, repoName)

		return e

	}); err != nil {

		slog.Warn("[scanner] TriggerRepoScan: failed to fetch metadata", "owner", owner, "repo", repoName, "error", err)

		return

	}

	files := s.scanRepo(repoCtx, owner, repoName)

	fullName := fmt.Sprintf("%s/%s", owner, repoName)

	s.mu.Lock()

	if len(files) > 0 {

		s.repos[fullName] = &RepoInfo{

			Name: fullName,

			FullName: ghRepo.GetFullName(),

			Description: ghRepo.GetDescription(),

			HTMLURL: ghRepo.GetHTMLURL(),

			Files: files,
		}

	} else {

		// Repo no longer has any indexed docs — remove from cache.

		delete(s.repos, fullName)

	}

	onComplete := s.onScanComplete

	s.mu.Unlock()

	if onComplete != nil {

		onComplete(s.ListRepos())

	}

}

// scanRepo checks a single repo for target documentation files.

func (s *Scanner) scanRepo(ctx context.Context, repoOwner, repoName string) []FileEntry {

	var entries []FileEntry

	// Check root-level target files.

	for _, target := range s.targetFiles {

		var fc *github.RepositoryContent

		var resp *github.Response

		err := retryGitHub(ctx, func() error {

			var e error

			fc, _, resp, e = s.client.Repositories.GetContents(ctx, repoOwner, repoName, target, nil)

			return e

		})

		if err != nil {

			if resp != nil && resp.StatusCode == 404 {

				continue

			}

			slog.Warn("[scanner] Error checking file", "repo", repoName, "target", target, "error", err)

			continue

		}

		if fc != nil {

			entries = append(entries, FileEntry{

				RepoName: repoName,

				Path: target,

				SHA: fc.GetSHA(),

				Type: s.classifyFile(target),
			})

		}

	}

	// Check configured directories recursively for .md files.

	for _, dir := range s.scanDirs {

		dirEntries := s.scanDocsDir(ctx, repoOwner, repoName, dir)

		entries = append(entries, dirEntries...)

	}

	// Check infra directories recursively for deployment/infrastructure files.

	for _, dir := range s.infraDirs {

		infraEntries := s.scanInfraDir(ctx, repoOwner, repoName, dir)

		entries = append(entries, infraEntries...)

	}

	return entries

}

// scanDocsDir recursively scans a directory for .md files.

func (s *Scanner) scanDocsDir(ctx context.Context, repoOwner, repoName, path string) []FileEntry {

	var entries []FileEntry

	var dirContents []*github.RepositoryContent

	var resp *github.Response

	err := retryGitHub(ctx, func() error {

		var e error

		_, dirContents, resp, e = s.client.Repositories.GetContents(ctx, repoOwner, repoName, path, nil)

		return e

	})

	if err != nil {

		if resp != nil && resp.StatusCode == 404 {

			return nil

		}

		slog.Warn("[scanner] Error scanning docs dir", "repo", repoName, "path", path, "error", err)

		return nil

	}

	for _, item := range dirContents {

		itemPath := item.GetPath()

		switch item.GetType() {

		case "file":

			lowerPath := strings.ToLower(itemPath)

			isAgentDir := strings.Contains(lowerPath, ".agents/") || strings.Contains(lowerPath, ".agent/") || strings.Contains(lowerPath, "_agents/") || strings.Contains(lowerPath, "_agent/")

			if strings.HasSuffix(lowerPath, ".md") || isAgentDir {

				entries = append(entries, FileEntry{

					RepoName: repoName,

					Path: itemPath,

					SHA: item.GetSHA(),

					Type: "docs",
				})

			}

		case "dir":

			// Recurse into subdirectories.

			subEntries := s.scanDocsDir(ctx, repoOwner, repoName, itemPath)

			entries = append(entries, subEntries...)

		}

	}

	return entries

}

// scanInfraDir recursively scans a directory for infrastructure and deployment files

// (.yaml, .yml, .tf, .hcl, .toml). Used for directories like deploy/, infra/, .github/workflows/.

func (s *Scanner) scanInfraDir(ctx context.Context, repoOwner, repoName, path string) []FileEntry {

	var entries []FileEntry

	var dirContents []*github.RepositoryContent

	var resp *github.Response

	err := retryGitHub(ctx, func() error {

		var e error

		_, dirContents, resp, e = s.client.Repositories.GetContents(ctx, repoOwner, repoName, path, nil)

		return e

	})

	if err != nil {

		if resp != nil && resp.StatusCode == 404 {

			return nil

		}

		slog.Warn("[scanner] Error scanning infra dir", "repo", repoName, "path", path, "error", err)

		return nil

	}

	for _, item := range dirContents {

		itemPath := item.GetPath()

		switch item.GetType() {

		case "file":

			ext := strings.ToLower(filepath.Ext(itemPath))

			if infraExtensions[ext] {

				entries = append(entries, FileEntry{

					RepoName: repoName,

					Path: itemPath,

					SHA: item.GetSHA(),

					Type: s.classifyFile(itemPath),
				})

			}

		case "dir":

			subEntries := s.scanInfraDir(ctx, repoOwner, repoName, itemPath)

			entries = append(entries, subEntries...)

		}

	}

	return entries

}

// classifyFile returns a type label for a given file path.

// It checks the scanner's parser registry first, then falls back to hardcoded rules

// for infra/docs types that have no associated FileParser.

func (s *Scanner) classifyFile(name string) string {

	base := filepath.Base(strings.ToLower(name))

	// Registry-based classification (exact filename match).

	for _, p := range s.registry.All() {

		for _, fn := range p.Filenames() {

			if strings.HasPrefix(fn, ".") && strings.HasSuffix(base, fn) {

				// Suffix match for extension-based parsers (e.g. ".proto").

				return p.FileType()

			}

			if base == strings.ToLower(fn) {

				return p.FileType()

			}

		}

	}

	lowerPath := strings.ToLower(name)

	switch base {

	// Documentation / catalog types not backed by a FileParser.

	case "mkdocs.yml":

		return "mkdocs"

	case "openapi.yaml":

		return "openapi"

	case "swagger.json":

		return "swagger"

	case "readme.md":

		return "readme"

	case "skills.md":

		return "skills"

	case "agents.md":

		return "agents"

	// Infrastructure / tooling

	case "dockerfile":

		return "dockerfile"

	case "makefile":

		return "makefile"

	case "docker-compose.yml", "docker-compose.yaml":

		return "compose"

	case ".mise.toml", "mise.toml":

		return "mise"

	// Helm

	case "chart.yaml":

		return "helm"

	case "values.yaml":

		if strings.Contains(lowerPath, "/helm/") {

			return "helm"

		}

		return "infra"

	}

	ext := filepath.Ext(base)

	switch ext {

	case ".tf", ".hcl":

		return "terraform"

	case ".yaml", ".yml":

		if strings.Contains(lowerPath, "/helm/") {

			return "helm"

		}

		if strings.Contains(lowerPath, "/k8s/") || strings.Contains(lowerPath, "/kubernetes/") {

			return "k8s"

		}

		if strings.Contains(lowerPath, "/workflows/") {

			return "workflow"

		}

		return "infra"

	case ".toml":

		return "toml"

	}

	return "docs"

}

// --- Public API used by MCP tools ---

// ListRepos returns all repos that have documentation files.

func (s *Scanner) ListRepos() []RepoInfo {

	s.mu.RLock()

	defer s.mu.RUnlock()

	result := make([]RepoInfo, 0, len(s.repos))

	for _, info := range s.repos {

		result = append(result, *info)

	}

	return result

}

// SearchDocs searches file paths and repo names for the given query term.

func (s *Scanner) SearchDocs(query string) []FileEntry {

	s.mu.RLock()

	defer s.mu.RUnlock()

	q := strings.ToLower(query)

	var results []FileEntry

	for _, info := range s.repos {

		for _, f := range info.Files {

			if strings.Contains(strings.ToLower(f.Path), q) ||

				strings.Contains(strings.ToLower(f.RepoName), q) {

				results = append(results, f)

			}

		}

	}

	return results

}

// IsIndexed checks if a specific file path is indexed in the cache as documentation.

func (s *Scanner) IsIndexed(repoName, path string) bool {

	s.mu.RLock()

	defer s.mu.RUnlock()

	repo, ok := s.repos[repoName]

	if !ok {

		return false

	}

	for _, f := range repo.Files {

		if strings.EqualFold(f.Path, path) {

			return true

		}

	}

	return false

}

// GetFileContent retrieves the raw content of a file from GitHub.

// For security (preventing path traversal to non-docs), it enforces that the file must be indexed.

func (s *Scanner) GetFileContent(ctx context.Context, repoName, path string) (string, error) {

	if !s.IsIndexed(repoName, path) {

		return "", fmt.Errorf("security policy: path '%s' is not indexed as a documentation file", path)

	}

	// We need to parse repoName to support EXTRA_REPOS format: "owner/repo"

	owner := s.org

	repo := repoName

	if strings.Contains(repoName, "/") {

		parts := strings.SplitN(repoName, "/", 2)

		owner = parts[0]

		repo = parts[1]

	}

	var fc *github.RepositoryContent

	var resp *github.Response

	err := retryGitHub(ctx, func() error {

		var e error

		fc, _, resp, e = s.client.Repositories.GetContents(ctx, owner, repo, path, nil)

		return e

	})

	if err != nil {

		if resp != nil && resp.StatusCode == 404 {

			return "", fmt.Errorf("file not found: %s/%s", repoName, path)

		}

		return "", fmt.Errorf("error fetching file %s/%s: %w", repoName, path, err)

	}

	if fc == nil {

		return "", fmt.Errorf("path is a directory, not a file: %s/%s", repoName, path)

	}

	content, err := fc.GetContent()

	if err != nil {

		return "", fmt.Errorf("error decoding content of %s/%s: %w", repoName, path, err)

	}

	return content, nil

}

// Status returns scanning status info.

func (s *Scanner) Status() (scanning bool, lastScan time.Time, repoCount int) {

	s.mu.RLock()

	defer s.mu.RUnlock()

	return s.scanning, s.lastScanAt, len(s.repos)

}
