// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package scanner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v60/github"
)

// DefaultTargetFiles defines the default filenames to look for at the repo root.
var DefaultTargetFiles = []string{
	"catalog-info.yaml",
	"mkdocs.yml",
	"openapi.yaml",
	"swagger.json",
	"README.md",
	"SKILLS.md",
	"AGENTS.md",
	"go.mod",
	"package.json",
	"pom.xml",
}

// DefaultScanDirs defines the default directories to scan recursively for .md files.
var DefaultScanDirs = []string{
	"docs",
	".agents",
}

// FileEntry represents an indexed documentation file.
type FileEntry struct {
	RepoName string `json:"repo_name"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
	Type     string `json:"type"` // e.g. "catalog-info", "mkdocs", "openapi", "swagger", "readme", "docs"
}

// RepoInfo holds metadata about a repository that contains documentation.
type RepoInfo struct {
	Name        string      `json:"name"`
	FullName    string      `json:"full_name"`
	Description string      `json:"description"`
	HTMLURL     string      `json:"html_url"`
	Files       []FileEntry `json:"files"`
}

// Scanner manages GitHub org scanning and caching.
type Scanner struct {
	client       *github.Client
	org          string
	scanInterval time.Duration
	targetFiles  []string       // files to look for at repo root
	scanDirs     []string       // directories to scan recursively for .md files
	extraRepos   []string       // extra explicit repos formatted as "owner/repo"
	repoTopics   []string       // filter org repos by topics
	repoRegex    *regexp.Regexp // filter org repos by name using regex

	mu    sync.RWMutex
	repos map[string]*RepoInfo // keyed by repo name

	scanning   bool
	lastScanAt time.Time

	onScanComplete func([]RepoInfo) // called after each full scan completes
}

// New creates a new Scanner instance.
func New(client *github.Client, org string, scanInterval time.Duration, targetFiles, scanDirs, extraRepos, repoTopics []string, repoRegex *regexp.Regexp) *Scanner {
	if len(targetFiles) == 0 {
		targetFiles = DefaultTargetFiles
	}
	if len(scanDirs) == 0 {
		scanDirs = DefaultScanDirs
	}
	return &Scanner{
		client:       client,
		org:          org,
		scanInterval: scanInterval,
		targetFiles:  targetFiles,
		scanDirs:     scanDirs,
		extraRepos:   extraRepos,
		repoTopics:   repoTopics,
		repoRegex:    repoRegex,
		repos:        make(map[string]*RepoInfo),
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
		log.Println("[scanner] Starting initial org scan...")
		s.scanOrg(ctx)
		log.Printf("[scanner] Initial scan complete. Found %d repos with docs.\n", len(s.repos))
	}()

	// Periodic re-scan.
	if s.scanInterval > 0 {
		go func() {
			ticker := time.NewTicker(s.scanInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					log.Println("[scanner] Starting periodic re-scan...")
					s.scanOrg(ctx)
					log.Printf("[scanner] Re-scan complete. Found %d repos with docs.\n", len(s.repos))
				}
			}
		}()
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
		log.Printf("[scanner] Error listing repos: %v\n", err)
		return
	}

	log.Printf("[scanner] Found %d total repos for %s\n", len(allRepos), s.org)

	// Start concurrent scanning with semaphore
	const maxConcurrency = 5
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	newRepos := make(map[string]*RepoInfo)
	var mu sync.Mutex

	for _, repo := range allRepos {
		// Filter by regex if set (only for org repos, extra repos via env aren't filtered here)
		// But in this loop, allRepos includes extraRepos. Let's filter everything consistently,
		// or maybe filter inside listAllRepos? It's cleaner to filter in listAllRepos.
		repoName := repo.GetName()
		wg.Add(1)
		sem <- struct{}{} // acquire

		go func(repoName string, repo *github.Repository) {
			defer wg.Done()
			defer func() { <-sem }() // release

			repoOwner := repo.GetOwner().GetLogin()

			// Per-repo timeout prevents a single slow GitHub response from stalling the scan.
			repoCtx, repoCancel := context.WithTimeout(ctx, 30*time.Second)
			defer repoCancel()

			files := s.scanRepo(repoCtx, repoOwner, repoName)
			if len(files) > 0 {
				info := &RepoInfo{
					Name:        fmt.Sprintf("%s/%s", repoOwner, repoName),
					FullName:    repo.GetFullName(),
					Description: repo.GetDescription(),
					HTMLURL:     repo.GetHTMLURL(),
					Files:       files,
				}
				mu.Lock()
				newRepos[info.Name] = info
				mu.Unlock()
			}
		}(repoName, repo)
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
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == 404 {
			log.Printf("[scanner] '%s' is not an org, trying as user account...\n", s.org)
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
			matchTopic := false
			for _, t := range r.Topics {
				for _, reqT := range s.repoTopics {
					if strings.EqualFold(t, reqT) {
						matchTopic = true
						break
					}
				}
				if matchTopic {
					break
				}
			}
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
			log.Printf("[scanner] Invalid EXTRA_REPOS format '%s', skipping\n", er)
			continue
		}
		var r *github.Repository
		if err := retryGitHub(ctx, func() error {
			var e error
			r, _, e = s.client.Repositories.Get(ctx, parts[0], parts[1])
			return e
		}); err != nil {
			log.Printf("[scanner] Error fetching extra repo %s: %v\n", er, err)
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
		log.Printf("[scanner] TriggerRepoScan: failed to fetch metadata for %s/%s: %v\n", owner, repoName, err)
		return
	}

	files := s.scanRepo(repoCtx, owner, repoName)
	fullName := fmt.Sprintf("%s/%s", owner, repoName)

	s.mu.Lock()
	if len(files) > 0 {
		s.repos[fullName] = &RepoInfo{
			Name:        fullName,
			FullName:    ghRepo.GetFullName(),
			Description: ghRepo.GetDescription(),
			HTMLURL:     ghRepo.GetHTMLURL(),
			Files:       files,
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
			log.Printf("[scanner] Error checking %s/%s: %v\n", repoName, target, err)
			continue
		}
		if fc != nil {
			entries = append(entries, FileEntry{
				RepoName: repoName,
				Path:     target,
				SHA:      fc.GetSHA(),
				Type:     classifyFile(target),
			})
		}
	}

	// Check configured directories recursively for .md files.
	for _, dir := range s.scanDirs {
		dirEntries := s.scanDocsDir(ctx, repoOwner, repoName, dir)
		entries = append(entries, dirEntries...)
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
		log.Printf("[scanner] Error scanning docs dir %s/%s: %v\n", repoName, path, err)
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
					Path:     itemPath,
					SHA:      item.GetSHA(),
					Type:     "docs",
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

// classifyFile returns a type label for a given filename.
func classifyFile(name string) string {
	base := filepath.Base(strings.ToLower(name))
	switch base {
	case "catalog-info.yaml": // Backstage
		return "catalog-info"
	case "mkdocs.yml": // MkDocs
		return "mkdocs"
	case "openapi.yaml": // OpenAPI
		return "openapi"
	case "swagger.json": // Swagger
		return "swagger"
	case "readme.md": // README
		return "readme"
	case "skills.md":
		return "skills"
	case "agents.md":
		return "agents"
	case "go.mod":
		return "gomod"
	case "package.json":
		return "packagejson"
	case "pom.xml":
		return "pomxml"
	default:
		return "docs" // Custom markdown files
	}
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
