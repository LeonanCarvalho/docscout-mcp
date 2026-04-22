// Copyright 2026 Doc Scout
// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuditEvent is the GORM model persisted to the audit_events table.
type AuditEvent struct {
	ID        string    `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	Agent     string
	Tool      string
	Operation string
	Targets   string // JSON array of entity/relation names
	Count     int
	Outcome   string // "ok" | "error"
	ErrorMsg  string
}

// AuditFilter constrains Query results.
type AuditFilter struct {
	Agent     string
	Tool      string
	Operation string
	Outcome   string
	Since     time.Time
	Limit     int
}

// AuditSummary is returned by AuditStore.Summary.
type AuditSummary struct {
	TotalMutations int
	ByAgent        map[string]int
	ByOperation    map[string]int
	ErrorRate      float64
	RiskyEvents    []AuditEvent
}

// AuditStore is the port used by GraphAuditLogger and the MCP tools.
type AuditStore interface {
	Write(ctx context.Context, e AuditEvent) error
	Query(ctx context.Context, f AuditFilter) ([]AuditEvent, int64, error)
	Summary(ctx context.Context, window time.Duration) (AuditSummary, error)
}

// DBAuditStore is the SQLite/Postgres implementation of AuditStore.
type DBAuditStore struct {
	db *gorm.DB
}

// NewAuditStore creates the audit_events table (if absent) and returns a store.
func NewAuditStore(db *gorm.DB) (*DBAuditStore, error) {
	if err := db.AutoMigrate(&AuditEvent{}); err != nil {
		return nil, err
	}
	return &DBAuditStore{db: db}, nil
}

// MarshalTargets serialises a slice of entity/relation names to a JSON string.
func MarshalTargets(names []string) string {
	b, _ := json.Marshal(names)
	return string(b)
}

// Write persists one audit event. A UUIDv7 primary key is generated here so
// ORDER BY id gives chronological order without an extra index.
func (s *DBAuditStore) Write(_ context.Context, e AuditEvent) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	e.ID = id.String()
	if res := s.db.Create(&e); res.Error != nil {
		slog.Error("audit write failed", "err", res.Error)
		return res.Error
	}
	return nil
}

// Query retrieves audit events matching the filter. Returns the page and total count.
func (s *DBAuditStore) Query(_ context.Context, f AuditFilter) ([]AuditEvent, int64, error) {
	q := s.db.Model(&AuditEvent{})
	if f.Agent != "" {
		q = q.Where("agent = ?", f.Agent)
	}
	if f.Tool != "" {
		q = q.Where("tool = ?", f.Tool)
	}
	if f.Operation != "" {
		q = q.Where("operation = ?", f.Operation)
	}
	if f.Outcome != "" {
		q = q.Where("outcome = ?", f.Outcome)
	}
	if !f.Since.IsZero() {
		q = q.Where("created_at >= ?", f.Since)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	} else if limit > 500 {
		limit = 500
	}

	var events []AuditEvent
	if err := q.Order("id asc").Limit(limit).Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// Summary returns anomaly-focused metrics over the given rolling window.
func (s *DBAuditStore) Summary(_ context.Context, window time.Duration) (AuditSummary, error) {
	since := time.Now().Add(-window)
	var events []AuditEvent
	if err := s.db.Where("created_at >= ?", since).Order("id asc").Find(&events).Error; err != nil {
		return AuditSummary{}, err
	}

	sum := AuditSummary{
		ByAgent:     make(map[string]int),
		ByOperation: make(map[string]int),
	}

	// Per-agent error window: track timestamps of errors to detect bursts.
	agentErrors := make(map[string][]time.Time)
	var errCount int

	for _, e := range events {
		sum.TotalMutations++
		sum.ByAgent[e.Agent]++
		sum.ByOperation[e.Operation]++

		if e.Outcome == "error" {
			errCount++
			agentErrors[e.Agent] = append(agentErrors[e.Agent], e.CreatedAt)
		}

		risky := false
		// Mass delete guard
		if e.Operation == "delete" && e.Count > 10 {
			risky = true
		}
		// Unknown agent
		if e.Agent == "unknown" {
			risky = true
		}
		if risky {
			sum.RiskyEvents = append(sum.RiskyEvents, e)
		}
	}

	// Error-burst check: > 5 errors from same agent within any 1-hour window.
	const burstLimit = 5
	burstWindow := time.Hour
	riskySet := make(map[string]bool)
	for _, re := range sum.RiskyEvents {
		riskySet[re.ID] = true
	}
	for agent, times := range agentErrors {
		if len(times) <= burstLimit {
			continue
		}
		for i := burstLimit; i < len(times); i++ {
			if times[i].Sub(times[i-burstLimit]) <= burstWindow {
				// Mark all events from this agent in the window as risky.
				for _, e := range events {
					if e.Agent == agent && e.Outcome == "error" && !riskySet[e.ID] {
						sum.RiskyEvents = append(sum.RiskyEvents, e)
						riskySet[e.ID] = true
					}
				}
				break
			}
		}
	}

	if sum.TotalMutations > 0 {
		sum.ErrorRate = float64(errCount) / float64(sum.TotalMutations)
	}

	return sum, nil
}
