package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/billstark001/latexmk/packages/cli/internal/protocol"
)

const remoteCleanupPlanKind = "remote"

// remoteCleanupPlan intentionally excludes authentication material. The
// caller must provide current credentials when applying the plan.
type remoteCleanupPlan struct {
	Version    int       `json:"version"`
	Kind       string    `json:"kind"`
	ID         string    `json:"planId"`
	Server     string    `json:"server"`
	ProjectID  string    `json:"projectId"`
	Scope      string    `json:"scope"`
	PlanDigest string    `json:"planDigest"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

type remoteCleanupOutput struct {
	PlanID    string                 `json:"planId"`
	ExpiresAt *time.Time             `json:"expiresAt,omitempty"`
	Report    protocol.CleanupReport `json:"report"`
}

func createRemoteCleanupPlan(server, projectID, scope string, report protocol.CleanupReport) (remoteCleanupPlan, error) {
	if report.ProjectID != projectID || report.Scope != scope || !report.DryRun {
		return remoteCleanupPlan{}, errors.New("server returned an inconsistent cleanup preview")
	}
	if !validCleanupPlanDigest(report.PlanDigest) {
		return remoteCleanupPlan{}, errors.New("server does not support atomic remote cleanup plans")
	}
	id, err := newCleanupPlanID()
	if err != nil {
		return remoteCleanupPlan{}, err
	}
	now := time.Now().UTC()
	plan := remoteCleanupPlan{
		Version: cleanupPlanVersion, Kind: remoteCleanupPlanKind, ID: id,
		Server: server, ProjectID: projectID, Scope: scope, PlanDigest: report.PlanDigest,
		CreatedAt: now, ExpiresAt: now.Add(cleanupPlanTTL),
	}
	if !validRemoteCleanupPlan(plan, id) {
		return remoteCleanupPlan{}, errors.New("remote cleanup plan contents are invalid")
	}
	if err := saveCleanupPlanData(plan.ID, plan); err != nil {
		return remoteCleanupPlan{}, err
	}
	return plan, nil
}

func loadRemoteCleanupPlan(planID string) (remoteCleanupPlan, string, error) {
	var plan remoteCleanupPlan
	path, err := loadCleanupPlanData(planID, &plan)
	if err != nil {
		return plan, path, err
	}
	if !validRemoteCleanupPlan(plan, planID) {
		return plan, path, errors.New("remote cleanup plan contents are invalid")
	}
	return plan, path, nil
}

func validRemoteCleanupPlan(plan remoteCleanupPlan, planID string) bool {
	if plan.Version != cleanupPlanVersion || plan.Kind != remoteCleanupPlanKind || plan.ID != planID {
		return false
	}
	if !validRemoteCleanupServer(plan.Server) || !validRemoteCleanupProjectID(plan.ProjectID) {
		return false
	}
	if plan.Scope != "results" && plan.Scope != "snapshot" && plan.Scope != "project" {
		return false
	}
	if !validCleanupPlanDigest(plan.PlanDigest) || plan.CreatedAt.IsZero() || plan.ExpiresAt.IsZero() {
		return false
	}
	duration := plan.ExpiresAt.Sub(plan.CreatedAt)
	return duration > 0 && duration <= cleanupPlanTTL
}

func validCleanupPlanDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validRemoteCleanupServer(value string) bool {
	if value == "" || value != strings.TrimRight(strings.TrimSpace(value), "/") {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" &&
		parsed.Opaque == "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func validRemoteCleanupProjectID(value string) bool {
	if len(value) == 0 || len(value) > 128 || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func consumeRemoteCleanupPlan(path string) error {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return errors.New("remote cleanup plan was already consumed")
		}
		return err
	}
	return nil
}
