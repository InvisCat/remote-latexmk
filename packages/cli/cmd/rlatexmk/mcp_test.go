package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/billstark001/latexmk/packages/cli/internal/client"
	"github.com/billstark001/latexmk/packages/cli/internal/config"
)

func testMCPClient(t *testing.T, root string) *client.Client {
	t.Helper()
	c, err := client.New("http://127.0.0.1:1", "secret-token-must-not-leak", time.Second, false, "")
	if err != nil {
		t.Fatal(err)
	}
	c.ProjectRoot = root
	c.Exclude = append(config.DefaultExcludes(), config.DefaultDeny()...)
	c.RespectGitIgnore = true
	c.UploadMode = "auto"
	return c
}

func TestMCPInitializeAndToolListUseJSONOnly(t *testing.T) {
	root := t.TempDir()
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n") + "\n"
	var stdout bytes.Buffer
	server := newStdioMCPServer(strings.NewReader(input), &stdout, root, testMCPClient(t, root), "xelatex", time.Second)
	if err := server.serve(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("responses = %d: %s", len(lines), stdout.String())
	}
	var initialized struct {
		Result struct {
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &initialized); err != nil {
		t.Fatal(err)
	}
	if initialized.Result.ServerInfo.Name != "remote-latexmk" {
		t.Fatalf("MCP server name = %q", initialized.Result.ServerInfo.Name)
	}
	for _, line := range lines {
		var response map[string]any
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("non-JSON stdout %q: %v", line, err)
		}
		if response["jsonrpc"] != "2.0" {
			t.Fatalf("response = %#v", response)
		}
	}
	var listed struct {
		Result struct {
			Tools []mcpTool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Result.Tools) != 13 {
		t.Fatalf("tools = %d: %s", len(listed.Result.Tools), lines[1])
	}
	for _, tool := range listed.Result.Tools {
		if tool.InputSchema["additionalProperties"] != false || tool.Annotations["openWorldHint"] != false {
			t.Fatalf("unsafe or open schema for %s: %#v %#v", tool.Name, tool.InputSchema, tool.Annotations)
		}
		if tool.Name == "job_cancel" && strings.Contains(tool.Description, "running") {
			t.Fatalf("job_cancel advertises unsupported running-job cancellation: %q", tool.Description)
		}
	}
}

func TestMCPToolCallEnvelopeAcceptsMetaWithoutWeakeningArguments(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.tex"), []byte("\\documentclass{article}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"project_entries","_meta":{"progressToken":"progress-1","example.test/value":{"nested":true}}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"project_manifest","arguments":{"entry":"main.tex"},"_meta":{"progressToken":7}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"project_manifest","arguments":{"entry":"main.tex"},"task":{"ttl":60000}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"project_manifest","arguments":{"entry":"main.tex","shellEscape":true}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"project_entries","arguments":{},"unexpected":true}}`,
	}, "\n") + "\n"
	var stdout bytes.Buffer
	server := newStdioMCPServer(strings.NewReader(input), &stdout, root, testMCPClient(t, root), "xelatex", time.Second)
	if err := server.serve(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("responses = %d: %s", len(lines), stdout.String())
	}

	decode := func(index int) (json.RawMessage, *mcpResponseError) {
		t.Helper()
		var response struct {
			Result json.RawMessage   `json:"result"`
			Error  *mcpResponseError `json:"error"`
		}
		if err := json.Unmarshal([]byte(lines[index]), &response); err != nil {
			t.Fatal(err)
		}
		return response.Result, response.Error
	}
	for _, index := range []int{1, 2, 3} {
		raw, responseErr := decode(index)
		if responseErr != nil {
			t.Fatalf("metadata call %d failed: %#v", index, responseErr)
		}
		var result mcpToolResult
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			t.Fatalf("metadata call %d returned tool error: %s", index, raw)
		}
	}
	if len(server.manifests) != 2 {
		t.Fatalf("successful manifest calls = %d, want 2", len(server.manifests))
	}
	raw, argumentErr := decode(4)
	if argumentErr != nil {
		t.Fatalf("unknown business argument became a protocol error: %#v", argumentErr)
	}
	var argumentResult mcpToolResult
	if err := json.Unmarshal(raw, &argumentResult); err != nil {
		t.Fatal(err)
	}
	if !argumentResult.IsError || !strings.Contains(string(raw), "unknown field") {
		t.Fatalf("strict tool argument response = %s", raw)
	}
	_, envelopeErr := decode(5)
	if envelopeErr == nil || envelopeErr.Code != -32602 {
		t.Fatalf("unknown envelope field error = %#v, want -32602", envelopeErr)
	}
	if len(server.manifests) != 2 {
		t.Fatalf("rejected calls changed manifest state: %d", len(server.manifests))
	}
}

func TestMCPServerStatusAcceptsCodexProgressMetadata(t *testing.T) {
	root := t.TempDir()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/v1/meta":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"service":"remote-latexmk","version":"test","protocolVersion":2}`))
		case "/v1/jobs":
			if r.Header.Get("Authorization") != "Bearer token" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jobs":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(remote.Close)
	c, err := client.New(remote.URL, "token", time.Second, false, "")
	if err != nil {
		t.Fatal(err)
	}
	c.ProjectRoot = root
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"codex","version":"test"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"server_status","arguments":{},"_meta":{"progressToken":"codex-progress"}}}`,
	}, "\n") + "\n"
	var stdout bytes.Buffer
	server := newStdioMCPServer(strings.NewReader(input), &stdout, root, c, "xelatex", time.Second)
	if err := server.serve(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("responses = %d: %s", len(lines), stdout.String())
	}
	var response struct {
		Result mcpToolResult     `json:"result"`
		Error  *mcpResponseError `json:"error"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error != nil || response.Result.IsError {
		t.Fatalf("server_status with progress metadata failed: %s", lines[1])
	}
	if !strings.Contains(lines[1], `"accessVerified":true`) {
		t.Fatalf("server_status did not verify authenticated access: %s", lines[1])
	}
}

func TestMCPRejectsCallsBeforeInitialize(t *testing.T) {
	root := t.TempDir()
	var stdout bytes.Buffer
	server := newStdioMCPServer(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`+"\n"), &stdout, root, testMCPClient(t, root), "xelatex", time.Second)
	if err := server.serve(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"code":-32002`) {
		t.Fatalf("response = %s", stdout.String())
	}
}

func TestMCPDiscoversAndFixesOneClientWorkspaceRoot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	root := t.TempDir()
	resolvedRoot, err := resolveMCPRoot("", root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.tex"), []byte("\\documentclass{article}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rootURI := testMCPFileURI(root)
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{"roots":{"listChanged":true}},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"remote-latexmk-roots-1","result":{"roots":[{"uri":"` + rootURI + `","name":"paper"}]}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"project_manifest","arguments":{"entry":"main.tex"}}}`,
	}, "\n") + "\n"
	var stdout bytes.Buffer
	server := newRootDiscoveringMCPServer(strings.NewReader(input), &stdout, "")
	if err := server.serve(); err != nil {
		t.Fatal(err)
	}
	if !server.runtimeReady || server.root != resolvedRoot || server.client == nil {
		t.Fatalf("discovered runtime ready=%t root=%q client=%v", server.runtimeReady, server.root, server.client)
	}
	if strings.Contains(stdout.String(), resolvedRoot) {
		t.Fatalf("MCP stdout leaked absolute root: %s", stdout.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 3 || !strings.Contains(lines[1], `"method":"roots/list"`) {
		t.Fatalf("root discovery transcript: %s", stdout.String())
	}
	var result struct {
		Result mcpToolResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[2]), &result); err != nil {
		t.Fatal(err)
	}
	if result.Result.IsError {
		t.Fatalf("manifest failed after root discovery: %s", lines[2])
	}
}

func TestMCPRootDiscoveryFailsClosedWithoutOneRoot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	rootA := t.TempDir()
	rootB := t.TempDir()
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{"roots":{}},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"remote-latexmk-roots-1","result":{"roots":[{"uri":"` + testMCPFileURI(rootA) + `"},{"uri":"` + testMCPFileURI(rootB) + `"}]}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"server_status","arguments":{}}}`,
	}, "\n") + "\n"
	var stdout bytes.Buffer
	server := newRootDiscoveringMCPServer(strings.NewReader(input), &stdout, rootA)
	if err := server.serve(); err != nil {
		t.Fatal(err)
	}
	if server.runtimeReady || server.root != "" {
		t.Fatalf("ambiguous roots were accepted: root=%q", server.root)
	}
	if !strings.Contains(stdout.String(), "requires exactly one workspace root") {
		t.Fatalf("ambiguous-root error missing: %s", stdout.String())
	}
}

func TestMCPRootDiscoveryRejectsProjectRootOutsideWorkspace(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	parent := t.TempDir()
	root := filepath.Join(parent, "paper")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.FileName), []byte(`{"projectRoot":".."}`), 0o600); err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{"roots":{}},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"remote-latexmk-roots-1","result":{"roots":[{"uri":"` + testMCPFileURI(root) + `"}]}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"server_status","arguments":{}}}`,
	}, "\n") + "\n"
	var stdout bytes.Buffer
	server := newRootDiscoveringMCPServer(strings.NewReader(input), &stdout, "")
	if err := server.serve(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "configured project root is outside the MCP workspace") {
		t.Fatalf("outside-root error missing: %s", stdout.String())
	}
}

func TestMCPUsesBoundedFallbackWhenClientDoesNotAdvertiseRoots(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	root := t.TempDir()
	resolvedRoot, err := resolveMCPRoot("", root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.tex"), []byte("\\documentclass{article}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"codex","version":"desktop"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"project_manifest","arguments":{"entry":"main.tex"}}}`,
	}, "\n") + "\n"
	var stdout bytes.Buffer
	server := newRootDiscoveringMCPServer(strings.NewReader(input), &stdout, resolvedRoot)
	if err := server.serve(); err != nil {
		t.Fatal(err)
	}
	if !server.runtimeReady || server.root != resolvedRoot || server.client == nil {
		t.Fatalf("fallback runtime ready=%t root=%q client=%v", server.runtimeReady, server.root, server.client)
	}
	if strings.Contains(stdout.String(), resolvedRoot) {
		t.Fatalf("MCP stdout leaked absolute fallback root: %s", stdout.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 || strings.Contains(stdout.String(), `"method":"roots/list"`) {
		t.Fatalf("fallback transcript: %s", stdout.String())
	}
	var result struct {
		Result mcpToolResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &result); err != nil {
		t.Fatal(err)
	}
	if result.Result.IsError {
		t.Fatalf("manifest failed with Codex cwd fallback: %s", lines[1])
	}
}

func testMCPFileURI(path string) string {
	slash := filepath.ToSlash(path)
	if runtime.GOOS == "windows" && !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return (&url.URL{Scheme: "file", Path: slash}).String()
}

func TestMCPManifestIsOneUseAndInvalidAfterSourceChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.tex")
	if err := os.WriteFile(path, []byte("\\documentclass{article}\n\\begin{document}first\\end{document}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newStdioMCPServer(&bytes.Buffer{}, &bytes.Buffer{}, root, testMCPClient(t, root), "xelatex", time.Second)
	data, err := server.toolProjectManifest(json.RawMessage(`{"entry":"main.tex"}`))
	if err != nil {
		t.Fatal(err)
	}
	manifestID, ok := data.(map[string]any)["manifestId"].(string)
	if !ok || manifestID == "" {
		t.Fatalf("manifest = %#v", data)
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("secret-token-must-not-leak")) || bytes.Contains(encoded, []byte(root)) {
		t.Fatalf("manifest leaked private configuration: %s", encoded)
	}
	if err := os.WriteFile(path, []byte("\\documentclass{article}\n\\begin{document}changed\\end{document}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"manifestId":"` + manifestID + `"}`)
	if _, err := server.toolCompileStart(raw); err == nil || !strings.Contains(err.Error(), "manifest changed") {
		t.Fatalf("change error = %v", err)
	}
	if _, err := server.toolCompileStart(raw); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("reuse error = %v", err)
	}
}

func TestMCPManifestExpiresAndOutputDirectoryIsConfined(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.tex"), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newStdioMCPServer(&bytes.Buffer{}, &bytes.Buffer{}, root, testMCPClient(t, root), "xelatex", time.Second)
	now := time.Now()
	server.now = func() time.Time { return now }
	data, err := server.toolProjectManifest(json.RawMessage(`{"entry":"main.tex"}`))
	if err != nil {
		t.Fatal(err)
	}
	manifestID := data.(map[string]any)["manifestId"].(string)
	now = now.Add(mcpManifestTTL + time.Second)
	if _, err := server.toolCompileStart(json.RawMessage(`{"manifestId":"` + manifestID + `"}`)); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expiry error = %v", err)
	}
	if _, err := resolveMCPOutputDir(root, "/tmp"); err == nil {
		t.Fatal("absolute output directory was accepted")
	}
	if _, err := resolveMCPOutputDir(root, "../outside"); err == nil {
		t.Fatal("escaping output directory was accepted")
	}
}

func TestMCPToolArgumentsRejectUnknownFields(t *testing.T) {
	root := t.TempDir()
	server := newStdioMCPServer(&bytes.Buffer{}, &bytes.Buffer{}, root, testMCPClient(t, root), "xelatex", time.Second)
	if _, err := server.toolProjectManifest(json.RawMessage(`{"entry":"main.tex","shellEscape":true}`)); err == nil {
		t.Fatal("unknown shellEscape field was accepted")
	}
}

func TestMCPRemoteCleanupUsesServerAtomicPlan(t *testing.T) {
	root := t.TempDir()
	projectID, err := client.ResolveProjectID(root, true)
	if err != nil {
		t.Fatal(err)
	}
	previewCalls := 0
	deleteCalls := 0
	digest := strings.Repeat("a", 64)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/meta":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"protocolVersion": 2,
				"capabilities":    map[string]any{"remoteCleanup": true},
			})
		case strings.Contains(r.URL.Path, "/cleanup") && r.Method == http.MethodGet:
			previewCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"projectId": projectID,
				"scope":     "results", "dryRun": true, "results": 1, "resultBytes": 10,
				"planDigest": digest,
			})
		case strings.Contains(r.URL.Path, "/cleanup") && r.Method == http.MethodDelete:
			deleteCalls++
			if r.URL.Query().Get("expectedDigest") != digest {
				t.Fatalf("expectedDigest = %q", r.URL.Query().Get("expectedDigest"))
			}
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "cleanup targets changed since preview; create a new plan"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	c, err := client.New(remote.URL, "token", time.Second, false, "")
	if err != nil {
		t.Fatal(err)
	}
	c.ProjectRoot = root
	server := newStdioMCPServer(&bytes.Buffer{}, &bytes.Buffer{}, root, c, "xelatex", time.Second)
	preview, err := server.previewRemoteCleanup("remote-results")
	if err != nil {
		t.Fatal(err)
	}
	planID := preview.(map[string]any)["planId"].(string)
	if _, err := server.applyRemoteCleanup(planID); err == nil || !strings.Contains(err.Error(), "changed since preview") {
		t.Fatalf("drift error = %v", err)
	}
	if previewCalls != 1 || deleteCalls != 1 {
		t.Fatalf("preview calls = %d, delete calls = %d", previewCalls, deleteCalls)
	}
}
