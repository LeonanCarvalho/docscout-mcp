// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

// Package webhook provides an HTTP handler for GitHub webhook events.
// It validates the HMAC-SHA256 signature sent by GitHub and triggers an
// incremental single-repo scan for push, create, delete, and repository events.
//
// The webhook endpoint is entirely optional: it is only registered when the
// GITHUB_WEBHOOK_SECRET environment variable is set. The periodic full-org scan
// continues to work regardless.
package webhook

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/go-github/v60/github"
)

// RepoScanner is the subset of scanner.Scanner used by the webhook handler.
type RepoScanner interface {
	TriggerRepoScan(ctx context.Context, owner, repo string)
}

// Handler returns an http.Handler that validates GitHub webhook payloads and
// triggers incremental repo scans. serverCtx is used as the parent context for
// background scans so they are cancelled on graceful server shutdown.
func Handler(serverCtx context.Context, secret []byte, sc RepoScanner) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		// ValidatePayload reads the body, verifies X-Hub-Signature-256 (SHA256 HMAC),
		// and returns the raw JSON payload on success.
		payload, err := github.ValidatePayload(r, secret)
		if err != nil {
			slog.Warn("[webhook] Rejected request: invalid signature", "remote", r.RemoteAddr, "error", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		eventType := github.WebHookType(r)
		event, err := github.ParseWebHook(eventType, payload)
		if err != nil {
			slog.Warn("[webhook] Failed to parse event payload", "event_type", eventType, "error", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		owner, repo := repoFromEvent(event)
		if owner == "" || repo == "" {
			// Event type not relevant (e.g. ping, star, issue) — acknowledge and ignore.
			w.WriteHeader(http.StatusOK)
			return
		}

		deliveryID := r.Header.Get("X-Github-Delivery")
		slog.Info("[webhook] Incremental scan triggered",
			"event", eventType,
			"repo", owner+"/"+repo,
			"delivery", deliveryID,
		)

		// Scan asynchronously so the HTTP response is returned immediately.
		// serverCtx ensures the goroutine is cancelled on shutdown.
		go sc.TriggerRepoScan(serverCtx, owner, repo)

		w.WriteHeader(http.StatusOK)
	})
}

// repoFromEvent extracts the repository owner and name from supported GitHub event types.
// Returns empty strings for event types that do not carry repository information we act on.
func repoFromEvent(event interface{}) (owner, repo string) {
	switch e := event.(type) {
	case *github.PushEvent:
		return e.GetRepo().GetOwner().GetLogin(), e.GetRepo().GetName()
	case *github.CreateEvent:
		return e.GetRepo().GetOwner().GetLogin(), e.GetRepo().GetName()
	case *github.DeleteEvent:
		return e.GetRepo().GetOwner().GetLogin(), e.GetRepo().GetName()
	case *github.RepositoryEvent:
		return e.GetRepo().GetOwner().GetLogin(), e.GetRepo().GetName()
	}
	return "", ""
}
