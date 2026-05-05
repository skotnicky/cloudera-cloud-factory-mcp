package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	mcp_golang "github.com/metoro-io/mcp-golang"
)

func resetMCPLockStateForTest() {
	globalMCPLockState.mu.Lock()
	defer globalMCPLockState.mu.Unlock()
	globalMCPLockState.hardLimitScope = nil
	globalMCPLockState.runtimeScope = nil
	globalMCPLockState.createdProjectIDs = nil
	mcpLockStateFilePath = ""
}

func parseResponseMap(t *testing.T, response *mcp_golang.ToolResponse) map[string]interface{} {
	t.Helper()
	if response == nil {
		t.Fatal("expected non-nil response")
	}
	if len(response.Content) == 0 || response.Content[0].TextContent == nil {
		t.Fatal("expected text content in tool response")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(response.Content[0].TextContent.Text), &decoded); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	return decoded
}

func TestParseMCPLockIDList(t *testing.T) {
	ids, err := parseMCPLockIDList(" 3,2,3,1 ")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	expected := []int32{1, 2, 3}
	if len(ids) != len(expected) {
		t.Fatalf("expected %d ids, got %d", len(expected), len(ids))
	}
	for i := range expected {
		if ids[i] != expected[i] {
			t.Fatalf("expected ids %v, got %v", expected, ids)
		}
	}
}

func TestInitMCPLockFromEnv(t *testing.T) {
	resetMCPLockStateForTest()

	statePath := filepath.Join(t.TempDir(), "mcp-lock-state.json")
	env := map[string]string{
		mcpLockOrgIDsEnv:     "10,20",
		mcpLockProjectIDsEnv: "300,400",
		mcpLockStatePathEnv:  statePath,
	}
	if err := initMCPLockFromEnv(func(key string) string { return env[key] }); err != nil {
		t.Fatalf("initMCPLockFromEnv returned error: %v", err)
	}

	effective, active := getEffectiveMCPLockScope()
	if !active {
		t.Fatal("expected env lock to be active")
	}
	if len(effective.OrganizationIDs) != 2 || effective.OrganizationIDs[0] != 10 || effective.OrganizationIDs[1] != 20 {
		t.Fatalf("unexpected org scope: %+v", effective.OrganizationIDs)
	}
	if len(effective.ProjectIDs) != 2 || effective.ProjectIDs[0] != 300 || effective.ProjectIDs[1] != 400 {
		t.Fatalf("unexpected project scope: %+v", effective.ProjectIDs)
	}
}

func TestParseMCPLockIDsFromArgs(t *testing.T) {
	org, project, err := parseMCPLockIDsFromArgs([]string{
		mcpLockOrgIDsArg + "=7,8",
		mcpLockProjectIDsArg, "101,102",
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if org != "7,8" {
		t.Fatalf("expected org ids 7,8 got %q", org)
	}
	if project != "101,102" {
		t.Fatalf("expected project ids 101,102 got %q", project)
	}
}

func TestInitMCPLockFromConfigArgsOverrideEnv(t *testing.T) {
	resetMCPLockStateForTest()
	statePath := filepath.Join(t.TempDir(), "mcp-lock-state.json")
	env := map[string]string{
		mcpLockOrgIDsEnv:     "1,2",
		mcpLockProjectIDsEnv: "10,20",
		mcpLockStatePathEnv:  statePath,
	}
	args := []string{
		mcpLockOrgIDsArg, "9",
		mcpLockProjectIDsArg + "=99,100",
	}

	if err := initMCPLockFromConfig(func(key string) string { return env[key] }, args); err != nil {
		t.Fatalf("initMCPLockFromConfig returned error: %v", err)
	}

	effective, active := getEffectiveMCPLockScope()
	if !active {
		t.Fatal("expected lock to be active")
	}
	if len(effective.OrganizationIDs) != 1 || effective.OrganizationIDs[0] != 9 {
		t.Fatalf("expected org scope [9], got %+v", effective.OrganizationIDs)
	}
	if len(effective.ProjectIDs) != 2 || effective.ProjectIDs[0] != 99 || effective.ProjectIDs[1] != 100 {
		t.Fatalf("expected project scope [99 100], got %+v", effective.ProjectIDs)
	}
}

func TestMCPLockRuntimeMustBeSubsetOfHardLimitAndClearFallsBack(t *testing.T) {
	resetMCPLockStateForTest()
	statePath := filepath.Join(t.TempDir(), "mcp-lock-state.json")

	if err := initMCPLockFromEnv(func(key string) string {
		if key == mcpLockProjectIDsEnv {
			return "111"
		}
		if key == mcpLockStatePathEnv {
			return statePath
		}
		return ""
	}); err != nil {
		t.Fatalf("init env lock: %v", err)
	}

	resp, err := mcpLock(MCPLockArgs{ProjectIDs: []int32{222}})
	if err != nil {
		t.Fatalf("mcpLock returned error: %v", err)
	}
	rejected := parseResponseMap(t, resp)
	if rejected["error"] == nil {
		t.Fatalf("expected out-of-hard-limit runtime lock to be rejected, got %v", rejected)
	}

	_, err = mcpLock(MCPLockArgs{ProjectIDs: []int32{111}})
	if err != nil {
		t.Fatalf("mcpLock (subset) returned error: %v", err)
	}

	effective, _ := getEffectiveMCPLockScope()
	if len(effective.ProjectIDs) != 1 || effective.ProjectIDs[0] != 111 {
		t.Fatalf("expected effective project scope [111], got %+v", effective.ProjectIDs)
	}

	_, err = mcpLockClear(MCPLockClearArgs{})
	if err != nil {
		t.Fatalf("mcpLockClear returned error: %v", err)
	}
	effective, active := getEffectiveMCPLockScope()
	if !active {
		t.Fatal("expected env lock to remain active after clear")
	}
	if len(effective.ProjectIDs) != 1 || effective.ProjectIDs[0] != 111 {
		t.Fatalf("expected env lock project 111 after clear, got %+v", effective.ProjectIDs)
	}
}

func TestEnforceMCPLockBlocksOutOfScopeProject(t *testing.T) {
	resetMCPLockStateForTest()
	mcpLockStateFilePath = filepath.Join(t.TempDir(), "mcp-lock-state.json")
	_, err := mcpLock(MCPLockArgs{ProjectIDs: []int32{100}})
	if err != nil {
		t.Fatalf("mcpLock returned error: %v", err)
	}

	denied := enforceMCPLock("create-standalone-vm", ProjectIDArgs{ProjectID: 200})
	if denied == nil {
		t.Fatal("expected out-of-scope project to be blocked")
	}
	resp := parseResponseMap(t, denied)
	if resp["error"] == nil {
		t.Fatalf("expected error response, got %v", resp)
	}
}

func TestEnforceMCPLockAllowsInScopeProjectAndPayloadExtraction(t *testing.T) {
	resetMCPLockStateForTest()
	mcpLockStateFilePath = filepath.Join(t.TempDir(), "mcp-lock-state.json")
	_, err := mcpLock(MCPLockArgs{ProjectIDs: []int32{300}})
	if err != nil {
		t.Fatalf("mcpLock returned error: %v", err)
	}

	if denied := enforceMCPLock("commit-project", ProjectIDArgs{ProjectID: 300}); denied != nil {
		t.Fatalf("expected in-scope project to pass, got %+v", denied)
	}

	payloadArgs := JSONPayloadArgs{Payload: `{"projectId": 301}`}
	if denied := enforceMCPLock("create-standalone-vm", payloadArgs); denied == nil {
		t.Fatal("expected payload projectId extraction to block out-of-scope project")
	}
}

func TestMCPLockStatusReturnsConsistentSnapshot(t *testing.T) {
	resetMCPLockStateForTest()
	globalMCPLockState.mu.Lock()
	hardLimit := newMCPLockScope([]int32{1}, []int32{100}, "hardLimit")
	runtime := newMCPLockScope([]int32{1}, []int32{100}, "runtime")
	globalMCPLockState.hardLimitScope = &hardLimit
	globalMCPLockState.runtimeScope = &runtime
	globalMCPLockState.mu.Unlock()

	resp, err := mcpLockStatus(MCPLockStatusArgs{})
	if err != nil {
		t.Fatalf("mcpLockStatus error: %v", err)
	}
	decoded := parseResponseMap(t, resp)

	hardLimitMap, ok := decoded["hardLimit"].(map[string]interface{})
	if !ok || hardLimitMap == nil {
		t.Fatalf("expected hardLimit map in status response, got %T", decoded["hardLimit"])
	}
	effective, ok := decoded["effective"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected effective map in status response, got %T", decoded["effective"])
	}
	if effective["source"] != "hardLimit+runtime" {
		t.Fatalf("expected effective source hardLimit+runtime, got %v", effective["source"])
	}
}

func TestEffectiveScopeIntersectsHardLimitAndRuntime(t *testing.T) {
	resetMCPLockStateForTest()
	globalMCPLockState.mu.Lock()
	hardLimit := newMCPLockScope([]int32{10, 20}, []int32{100, 200}, "hardLimit")
	runtime := newMCPLockScope([]int32{20, 30}, []int32{200, 300}, "runtime")
	globalMCPLockState.hardLimitScope = &hardLimit
	globalMCPLockState.runtimeScope = &runtime
	globalMCPLockState.mu.Unlock()

	effective, active := getEffectiveMCPLockScope()
	if !active {
		t.Fatal("expected active effective scope")
	}
	if len(effective.OrganizationIDs) != 1 || effective.OrganizationIDs[0] != 20 {
		t.Fatalf("expected org intersection [20], got %+v", effective.OrganizationIDs)
	}
	if len(effective.ProjectIDs) != 1 || effective.ProjectIDs[0] != 200 {
		t.Fatalf("expected project intersection [200], got %+v", effective.ProjectIDs)
	}
}

func TestEnforceMCPLockOrgOnlyResolvesProjectOrgAndBlocksMismatch(t *testing.T) {
	resetMCPLockStateForTest()
	mcpLockStateFilePath = filepath.Join(t.TempDir(), "mcp-lock-state.json")
	_, err := mcpLock(MCPLockArgs{OrganizationIDs: []int32{10}})
	if err != nil {
		t.Fatalf("mcpLock returned error: %v", err)
	}

	originalResolver := resolveMCPLockOrgsForProjectsFn
	resolveMCPLockOrgsForProjectsFn = func(projectIDs []int32) ([]int32, error) {
		if len(projectIDs) != 1 || projectIDs[0] != 200 {
			return nil, fmt.Errorf("unexpected projectIDs: %v", projectIDs)
		}
		return []int32{20}, nil
	}
	defer func() {
		resolveMCPLockOrgsForProjectsFn = originalResolver
	}()

	denied := enforceMCPLock("commit-project", ProjectIDArgs{ProjectID: 200})
	if denied == nil {
		t.Fatal("expected org-only lock to block project outside allowed organization")
	}
}

func TestEnforceMCPLockOrgOnlyAllowsResolvedMatch(t *testing.T) {
	resetMCPLockStateForTest()
	mcpLockStateFilePath = filepath.Join(t.TempDir(), "mcp-lock-state.json")
	_, err := mcpLock(MCPLockArgs{OrganizationIDs: []int32{10}})
	if err != nil {
		t.Fatalf("mcpLock returned error: %v", err)
	}

	originalResolver := resolveMCPLockOrgsForProjectsFn
	resolveMCPLockOrgsForProjectsFn = func(projectIDs []int32) ([]int32, error) {
		return []int32{10}, nil
	}
	defer func() {
		resolveMCPLockOrgsForProjectsFn = originalResolver
	}()

	if denied := enforceMCPLock("commit-project", ProjectIDArgs{ProjectID: 200}); denied != nil {
		t.Fatalf("expected resolved matching organization to pass, got %+v", denied)
	}
}

func TestEnforceMCPLockUnrestrictedWhenNoLocksConfigured(t *testing.T) {
	resetMCPLockStateForTest()
	if denied := enforceMCPLock("list-servers", ProjectIDArgs{ProjectID: 999}); denied != nil {
		t.Fatalf("expected unrestricted call to pass when no locks configured, got %+v", denied)
	}
}

func TestCreatedProjectIDsExtendEffectiveHardLimit(t *testing.T) {
	resetMCPLockStateForTest()
	globalMCPLockState.mu.Lock()
	hardLimit := newMCPLockScope(nil, []int32{1832}, "hardLimit")
	globalMCPLockState.hardLimitScope = &hardLimit
	globalMCPLockState.createdProjectIDs = []int32{3169}
	globalMCPLockState.mu.Unlock()

	effective, active := getEffectiveMCPLockScope()
	if !active {
		t.Fatal("expected active lock scope")
	}
	if len(effective.ProjectIDs) != 2 || effective.ProjectIDs[0] != 1832 || effective.ProjectIDs[1] != 3169 {
		t.Fatalf("expected project union [1832 3169], got %+v", effective.ProjectIDs)
	}
}

func TestPersistedCreatedProjectIDsRoundtrip(t *testing.T) {
	resetMCPLockStateForTest()
	path := filepath.Join(t.TempDir(), "lock-state.json")
	mcpLockStateFilePath = path
	globalMCPLockState.mu.Lock()
	globalMCPLockState.createdProjectIDs = []int32{4001, 4002}
	globalMCPLockState.mu.Unlock()

	if err := saveMCPLockState(); err != nil {
		t.Fatalf("saveMCPLockState error: %v", err)
	}
	loaded, err := loadPersistedMCPLockState(path)
	if err != nil {
		t.Fatalf("loadPersistedMCPLockState error: %v", err)
	}
	if len(loaded.CreatedProjectIDs) != 2 || loaded.CreatedProjectIDs[0] != 4001 || loaded.CreatedProjectIDs[1] != 4002 {
		t.Fatalf("unexpected persisted created IDs: %+v", loaded.CreatedProjectIDs)
	}
}

func TestUpdateCreatedProjectAllowlistFromCreateProjectResponse(t *testing.T) {
	resetMCPLockStateForTest()
	mcpLockStateFilePath = filepath.Join(t.TempDir(), "lock-state.json")
	response := createJSONResponse(map[string]interface{}{
		"success": true,
		"id":      "4501",
	})

	updateCreatedProjectAllowlistAfterTool("create-project", ProjectIDArgs{ProjectID: 1}, response, nil)
	created := getCreatedProjectIDs()
	if len(created) != 1 || created[0] != 4501 {
		t.Fatalf("expected created project [4501], got %+v", created)
	}

	content, err := os.ReadFile(mcpLockStateFilePath)
	if err != nil {
		t.Fatalf("expected persisted state file: %v", err)
	}
	var persisted mcpLockPersistedState
	if err := json.Unmarshal(content, &persisted); err != nil {
		t.Fatalf("invalid persisted state JSON: %v", err)
	}
	if len(persisted.CreatedProjectIDs) != 1 || persisted.CreatedProjectIDs[0] != 4501 {
		t.Fatalf("unexpected persisted project IDs: %+v", persisted.CreatedProjectIDs)
	}
}

func TestUpdateCreatedProjectAllowlistFromCreateVirtualClusterArgs(t *testing.T) {
	resetMCPLockStateForTest()
	mcpLockStateFilePath = filepath.Join(t.TempDir(), "lock-state.json")
	args := struct {
		ProjectID int32 `json:"projectId"`
	}{
		ProjectID: 7777,
	}

	updateCreatedProjectAllowlistAfterTool("create-virtual-cluster", args, createJSONResponse(SuccessResponse{Success: true, Message: "ok"}), nil)
	created := getCreatedProjectIDs()
	if len(created) != 1 || created[0] != 7777 {
		t.Fatalf("expected created project [7777], got %+v", created)
	}
}
