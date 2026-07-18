package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/billstark001/latexmk/packages/cli/internal/client"
	"github.com/billstark001/latexmk/packages/cli/internal/config"
	"github.com/billstark001/latexmk/packages/cli/internal/dependency"
)

const (
	cleanupPlanVersion = 1
	cleanupPlanTTL     = 10 * time.Minute
	maxCleanupTargets  = 20_000
	maxCleanupPlans    = 64
	maxCleanupFileSize = 512 << 20
	maxCleanupBytes    = 2 << 30
)

var (
	cleanupPlanIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	cleanupPlansDir      = defaultCleanupPlansDir
)

type cleanupTarget struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type cleanupPlan struct {
	Version     int             `json:"version"`
	ID          string          `json:"planId"`
	ProjectRoot string          `json:"projectRoot"`
	Scope       string          `json:"scope"`
	CreatedAt   time.Time       `json:"createdAt"`
	ExpiresAt   time.Time       `json:"expiresAt"`
	Targets     []cleanupTarget `json:"targets"`
	TotalBytes  int64           `json:"totalBytes"`
}

type cleanupResult struct {
	cleanupPlan
	DryRun    bool  `json:"dryRun"`
	Removed   int   `json:"removed"`
	Reclaimed int64 `json:"reclaimedBytes"`
}

type generatedCacheInfo struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

type localCacheInspection struct {
	ProjectRoot string               `json:"projectRoot"`
	ProjectID   string               `json:"projectId,omitempty"`
	Dependency  dependency.CacheInfo `json:"dependencyCache"`
	Generated   generatedCacheInfo   `json:"localGenerated"`
}

type cacheCommandOptions struct {
	projectRoot string
	scope       string
	planID      string
	yes         bool
	dryRun      bool
	jsonOutput  bool
}

func runCache(args []string) int {
	jsonOutput := hasJSONFlag(args)
	if len(args) == 0 {
		return failAgentArguments("cache", jsonOutput, errors.New("cache requires inspect or clean"))
	}
	action := args[0]
	if action != "inspect" && action != "clean" {
		return failAgentArguments("cache."+action, jsonOutput, fmt.Errorf("unknown cache action %q", action))
	}
	cwd, err := os.Getwd()
	if err != nil {
		return failAgent("cache."+action, jsonOutput, err)
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		return failAgent("cache."+action, jsonOutput, err)
	}
	opts := cacheCommandOptions{projectRoot: cfg.ProjectRoot, dryRun: true, jsonOutput: jsonOutput}
	if err := parseCacheArgs(action, args[1:], &opts); err != nil {
		return failAgentArguments("cache."+action, jsonOutput, err)
	}
	root, err := resolveCacheRoot(cwd, opts.projectRoot)
	if err != nil {
		return failAgent("cache."+action, opts.jsonOutput, err)
	}
	if action == "inspect" {
		inspection, err := inspectLocalCache(root)
		if err != nil {
			return failAgent("cache.inspect", opts.jsonOutput, err)
		}
		if opts.jsonOutput {
			if err := writeAgentJSON("cache.inspect", inspection); err != nil {
				return fail(err)
			}
			return 0
		}
		fmt.Printf("project root: %s\nproject ID: %s\ndependency cache: %t (%d bytes, %d entries)\ngenerated files: %d (%d bytes)\n", inspection.ProjectRoot, inspection.ProjectID, inspection.Dependency.Present, inspection.Dependency.Size, len(inspection.Dependency.Entries), inspection.Generated.Files, inspection.Generated.Bytes)
		return 0
	}
	if opts.yes {
		result, err := applyLocalCleanupPlan(root, opts.planID)
		if err != nil {
			return failAgent("cache.clean.apply", opts.jsonOutput, err)
		}
		if opts.jsonOutput {
			if err := writeAgentJSON("cache.clean.apply", result); err != nil {
				return fail(err)
			}
			return 0
		}
		fmt.Printf("plan: %s\nremoved: %d\nreclaimed bytes: %d\n", result.ID, result.Removed, result.Reclaimed)
		return 0
	}
	plan, err := createLocalCleanupPlan(root, opts.scope)
	if err != nil {
		return failAgent("cache.clean.preview", opts.jsonOutput, err)
	}
	result := cleanupResult{cleanupPlan: plan, DryRun: true}
	if opts.jsonOutput {
		if err := writeAgentJSON("cache.clean.preview", result); err != nil {
			return fail(err)
		}
		return 0
	}
	fmt.Printf("plan: %s\nscope: %s\ntargets: %d\nbytes: %d\nexpires: %s\npreview only; apply with --plan-id %s --yes\n", plan.ID, plan.Scope, len(plan.Targets), plan.TotalBytes, plan.ExpiresAt.Format(time.RFC3339), plan.ID)
	return 0
}

func parseCacheArgs(action string, args []string, opts *cacheCommandOptions) error {
	for i := 0; i < len(args); i++ {
		a := args[i]
		value := func(name string) (string, error) {
			if strings.Contains(a, "=") {
				return strings.SplitN(a, "=", 2)[1], nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", name)
			}
			i++
			return args[i], nil
		}
		var err error
		switch {
		case a == "--project-root" || strings.HasPrefix(a, "--project-root="):
			opts.projectRoot, err = value("--project-root")
		case a == "--scope" || strings.HasPrefix(a, "--scope="):
			opts.scope, err = value("--scope")
		case a == "--plan-id" || strings.HasPrefix(a, "--plan-id="):
			opts.planID, err = value("--plan-id")
		case a == "--yes":
			opts.yes = true
		case a == "--dry-run":
			opts.dryRun = true
		case a == "--json":
			opts.jsonOutput = true
		default:
			return fmt.Errorf("unknown option %q", a)
		}
		if err != nil {
			return err
		}
	}
	if action == "inspect" {
		if opts.scope != "" || opts.planID != "" || opts.yes || opts.dryRun {
			return errors.New("cache inspect accepts only --project-root and --json")
		}
		return nil
	}
	if opts.yes {
		if opts.dryRun {
			return errors.New("--yes and --dry-run cannot be used together")
		}
		if !cleanupPlanIDPattern.MatchString(opts.planID) {
			return errors.New("cache clean --yes requires a valid --plan-id from a preview")
		}
		if opts.scope != "" {
			return errors.New("do not pass --scope when applying a cleanup plan")
		}
		return nil
	}
	if opts.planID != "" {
		return errors.New("--plan-id requires --yes")
	}
	if opts.scope != "local-generated" && opts.scope != "local-client-cache" {
		return errors.New("--scope must be local-generated or local-client-cache")
	}
	return nil
}

func resolveCacheRoot(cwd, configured string) (string, error) {
	root := configured
	if root == "" {
		root = cwd
	} else if !filepath.IsAbs(root) {
		root = filepath.Join(cwd, root)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("project root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("project root is not a directory")
	}
	return resolved, nil
}

func inspectLocalCache(root string) (localCacheInspection, error) {
	dependencyInfo, err := dependency.InspectCache(root)
	if err != nil {
		return localCacheInspection{}, err
	}
	generated, err := collectCleanupTargets(root, "local-generated")
	if err != nil {
		return localCacheInspection{}, err
	}
	projectID, err := client.ResolveProjectID(root, false)
	if errors.Is(err, client.ErrProjectIDNotFound) {
		projectID = ""
	} else if err != nil {
		return localCacheInspection{}, err
	}
	var generatedBytes int64
	for _, target := range generated {
		generatedBytes += target.Size
	}
	return localCacheInspection{
		ProjectRoot: root, ProjectID: projectID, Dependency: dependencyInfo,
		Generated: generatedCacheInfo{Files: len(generated), Bytes: generatedBytes},
	}, nil
}

func createLocalCleanupPlan(root, scope string) (cleanupPlan, error) {
	targets, err := collectCleanupTargets(root, scope)
	if err != nil {
		return cleanupPlan{}, err
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return cleanupPlan{}, err
	}
	now := time.Now().UTC()
	plan := cleanupPlan{
		Version: cleanupPlanVersion, ID: hex.EncodeToString(idBytes), ProjectRoot: root,
		Scope: scope, CreatedAt: now, ExpiresAt: now.Add(cleanupPlanTTL), Targets: targets,
	}
	for _, target := range targets {
		plan.TotalBytes += target.Size
	}
	if err := saveCleanupPlan(plan); err != nil {
		return cleanupPlan{}, err
	}
	return plan, nil
}

func applyLocalCleanupPlan(root, planID string) (cleanupResult, error) {
	plan, planPath, err := loadCleanupPlan(planID)
	if err != nil {
		return cleanupResult{}, err
	}
	if time.Now().After(plan.ExpiresAt) {
		return cleanupResult{}, errors.New("cleanup plan has expired; create a new preview")
	}
	if plan.ProjectRoot != root {
		return cleanupResult{}, errors.New("cleanup plan belongs to a different project root")
	}
	for _, target := range plan.Targets {
		current, err := inspectCleanupTarget(root, target.Path)
		if err != nil {
			return cleanupResult{}, fmt.Errorf("cleanup target changed since preview: %s: %w", target.Path, err)
		}
		if current.Size != target.Size || current.SHA256 != target.SHA256 {
			return cleanupResult{}, fmt.Errorf("cleanup target changed since preview: %s", target.Path)
		}
	}
	result := cleanupResult{cleanupPlan: plan}
	for _, target := range plan.Targets {
		path, err := safeCleanupPath(root, target.Path)
		if err != nil {
			return result, err
		}
		if err := os.Remove(path); err != nil {
			return result, fmt.Errorf("remove %s: %w", target.Path, err)
		}
		result.Removed++
		result.Reclaimed += target.Size
	}
	if err := os.Remove(planPath); err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("remove applied cleanup plan: %w", err)
	}
	return result, nil
}

func collectCleanupTargets(root, scope string) ([]cleanupTarget, error) {
	if scope == "local-client-cache" {
		path := filepath.Join(root, ".latexmk-cache", "dependencies.json")
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return []cleanupTarget{}, nil
		} else if err != nil {
			return nil, err
		}
		target, err := inspectCleanupTarget(root, ".latexmk-cache/dependencies.json")
		if err != nil {
			return nil, err
		}
		return []cleanupTarget{target}, nil
	}
	if scope != "local-generated" {
		return nil, errors.New("unsupported local cleanup scope")
	}
	targets := make([]cleanupTarget, 0, 32)
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".latexmk-cache", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !generatedFileName(entry.Name()) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		target, err := inspectCleanupTarget(root, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		total += target.Size
		if len(targets) >= maxCleanupTargets || total > maxCleanupBytes {
			return errors.New("local cleanup exceeds safety limits")
		}
		targets = append(targets, target)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Path < targets[j].Path })
	return targets, nil
}

func generatedFileName(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range []string{".aux", ".bbl", ".bcf", ".blg", ".fdb_latexmk", ".fls", ".log", ".out", ".run.xml", ".synctex.gz", ".toc", ".xdv"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func inspectCleanupTarget(root, relative string) (cleanupTarget, error) {
	path, err := safeCleanupPath(root, relative)
	if err != nil {
		return cleanupTarget{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return cleanupTarget{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return cleanupTarget{}, errors.New("cleanup target is not a regular file")
	}
	if info.Size() < 0 || info.Size() > maxCleanupFileSize {
		return cleanupTarget{}, errors.New("cleanup target exceeds the per-file safety limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return cleanupTarget{}, err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, io.LimitReader(f, maxCleanupFileSize+1))
	closeErr := f.Close()
	if copyErr != nil {
		return cleanupTarget{}, copyErr
	}
	if closeErr != nil {
		return cleanupTarget{}, closeErr
	}
	return cleanupTarget{Path: filepath.ToSlash(filepath.Clean(relative)), Size: info.Size(), SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func safeCleanupPath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", errors.New("cleanup target path is invalid")
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("cleanup target escapes project root")
	}
	path := filepath.Join(root, clean)
	parent := filepath.Dir(path)
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedParent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("cleanup target parent escapes project root")
	}
	return filepath.Join(resolvedParent, filepath.Base(path)), nil
}

func defaultCleanupPlansDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "latexmk", "cleanup-plans"), nil
}

func saveCleanupPlan(plan cleanupPlan) error {
	dir, err := cleanupPlansDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("cleanup plan directory is not a real directory")
	}
	if err := pruneCleanupPlans(dir, time.Now()); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	path := filepath.Join(dir, plan.ID+".json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(payload); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return err
	}
	return f.Close()
}

func pruneCleanupPlans(dir string, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	active := 0
	for _, entry := range entries {
		name := entry.Name()
		id := strings.TrimSuffix(name, ".json")
		if name == id || !cleanupPlanIDPattern.MatchString(id) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && now.Sub(info.ModTime()) > cleanupPlanTTL+time.Minute {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		active++
	}
	if active >= maxCleanupPlans {
		return errors.New("too many active cleanup plans; wait for old plans to expire")
	}
	return nil
}

func loadCleanupPlan(planID string) (cleanupPlan, string, error) {
	var plan cleanupPlan
	if !cleanupPlanIDPattern.MatchString(planID) {
		return plan, "", errors.New("cleanup plan ID is invalid")
	}
	dir, err := cleanupPlansDir()
	if err != nil {
		return plan, "", err
	}
	path := filepath.Join(dir, planID+".json")
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return plan, path, errors.New("cleanup plan was not found; create a new preview")
		}
		return plan, path, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return plan, path, errors.New("cleanup plan file is invalid")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return plan, path, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return plan, path, fmt.Errorf("parse cleanup plan: %w", err)
	}
	if plan.Version != cleanupPlanVersion || plan.ID != planID || (plan.Scope != "local-generated" && plan.Scope != "local-client-cache") || len(plan.Targets) > maxCleanupTargets {
		return plan, path, errors.New("cleanup plan contents are invalid")
	}
	return plan, path, nil
}
