# Copyright 2026 Leonan Carvalho
# SPDX-License-Identifier: AGPL-3.0-only

variable "namespace" {
  description = "Kubernetes namespace to deploy DocScout-MCP into."
  type        = string
  default     = "docscout-mcp"
}

variable "image_tag" {
  description = "Docker image tag to deploy."
  type        = string
  default     = "latest"
}

variable "image_repository" {
  description = "Docker image repository."
  type        = string
  default     = "ghcr.io/doc-scout/mcp-server"
}

# ── Required secrets ──────────────────────────────────────────────────────────

variable "github_token" {
  description = "GitHub Personal Access Token (Fine-Grained, read-only Contents + Metadata)."
  type        = string
  sensitive   = true
}

# ── Required config ────────────────────────────────────────────────────────────

variable "github_org" {
  description = "GitHub Organization or User name to scan."
  type        = string
}

# ── Optional config ────────────────────────────────────────────────────────────

variable "scan_interval" {
  description = "Re-scan interval. Supports Go duration format (e.g. 30m, 1h) or plain integer minutes."
  type        = string
  default     = "30m"
}

variable "scan_files" {
  description = "Comma-separated list of filenames to scan at repo root. Leave empty for defaults."
  type        = string
  default     = ""
}

variable "scan_dirs" {
  description = "Comma-separated list of directories to scan recursively for .md files."
  type        = string
  default     = ""
}

variable "extra_repos" {
  description = "Comma-separated list of extra repos to scan (format: owner/repo)."
  type        = string
  default     = ""
}

variable "repo_topics" {
  description = "Filter org repos by GitHub topics (comma-separated)."
  type        = string
  default     = ""
}

variable "repo_regex" {
  description = "Filter org repos by regex matching the repo name (e.g. ^srv-.*)."
  type        = string
  default     = ""
}

variable "database_url" {
  description = "Knowledge graph storage URL. Accepts sqlite:///path or postgres://user:pass@host/db."
  type        = string
  default     = "sqlite:///data/docscout.db"
}

variable "scan_content" {
  description = "Enable content caching (requires persistent DATABASE_URL)."
  type        = bool
  default     = false
}

variable "max_content_size" {
  description = "Maximum content size in bytes to cache per file."
  type        = number
  default     = 204800
}

variable "mcp_http_bearer_token" {
  description = "Optional Bearer token for HTTP endpoint authentication."
  type        = string
  sensitive   = true
  default     = ""
}

variable "github_webhook_secret" {
  description = "Optional secret to enable the /webhook endpoint for incremental scans."
  type        = string
  sensitive   = true
  default     = ""
}

# ── Service / Ingress ──────────────────────────────────────────────────────────

variable "service_type" {
  description = "Kubernetes Service type (ClusterIP, LoadBalancer, NodePort)."
  type        = string
  default     = "ClusterIP"
}

variable "ingress_enabled" {
  description = "Whether to create an Ingress resource."
  type        = bool
  default     = false
}

variable "ingress_host" {
  description = "Hostname for the Ingress resource."
  type        = string
  default     = "docscout.example.com"
}

variable "ingress_class_name" {
  description = "IngressClass name (e.g. nginx, traefik)."
  type        = string
  default     = "nginx"
}

# ── Resources ─────────────────────────────────────────────────────────────────

variable "resources_requests_cpu" {
  type    = string
  default = "100m"
}

variable "resources_requests_memory" {
  type    = string
  default = "128Mi"
}

variable "resources_limits_cpu" {
  type    = string
  default = "500m"
}

variable "resources_limits_memory" {
  type    = string
  default = "512Mi"
}

variable "pvc_size" {
  description = "PersistentVolumeClaim storage size for the SQLite database volume."
  type        = string
  default     = "1Gi"
}

variable "pvc_storage_class" {
  description = "StorageClass for the PVC. Leave empty to use the cluster default."
  type        = string
  default     = ""
}
