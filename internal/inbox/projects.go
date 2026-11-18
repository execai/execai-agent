package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// A project (workspace) lives in integrations-vbai: a set of tools with
// profiles — the "PROJECT" selector of the web chat. The agent does not copy
// it; it reads the list and remembers which local directory belongs to which
// project (Binding), so that a task arriving with a workspace_id runs in the
// right place.
//
// The agent itself is a tool of the project: a workspace_tools row with
// tool_id "agent" and the agent_id in profile. That gives the on/off switch
// and the project card for free. agent_id, not the alias: the alias can be
// renamed in the settings and the binding would break.

// AgentToolID — the tool_id the agent lives under in workspace_tools. Must
// match the catalog entry in integrations-vbai.
const AgentToolID = "agent"

// WorkspaceTool is a tool row of a project.
type WorkspaceTool struct {
	// ID addresses the row for PUT and DELETE in integrations-vbai.
	ID      int    `json:"id"`
	ToolID  string `json:"tool_id"`
	Profile string `json:"profile"`
	Enabled bool   `json:"enabled"`
}

// Workspace is a project from integrations-vbai.
type Workspace struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	IsDefault bool            `json:"is_default"`
	Tools     []WorkspaceTool `json:"tools"`
}

// Workspaces lists the account's projects.
func (c *Client) Workspaces(ctx context.Context) ([]Workspace, error) {
	data, err := c.do(ctx, http.MethodGet, "/integrations-vbai/workspaces", nil, apiTimeout)
	if err != nil {
		return nil, err
	}
	var list []Workspace
	if err := json.Unmarshal(data, &list); err == nil {
		return list, nil
	}
	// Some versions wrap the list in an object.
	var wrapped struct {
		Workspaces []Workspace `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("inbox.err.parse"), err)
	}
	return wrapped.Workspaces, nil
}

// FindAgentTool finds this agent's row among the project's tools. The second
// value says whether it exists: the row itself may be switched off.
func FindAgentTool(w Workspace, agentID string) (WorkspaceTool, bool) {
	for _, t := range w.Tools {
		if t.ToolID == AgentToolID && t.Profile == agentID {
			return t, true
		}
	}
	return WorkspaceTool{}, false
}

// AddAgent adds the agent to the project as a tool, switched on. Idempotent
// on our side: integrations-vbai does not deduplicate, and every repeated
// bind would otherwise show the same machine once more in the project card.
func (c *Client) AddAgent(ctx context.Context, w Workspace, agentID string) error {
	if _, ok := FindAgentTool(w, agentID); ok {
		return nil
	}
	body, _ := json.Marshal(map[string]any{"tool_id": AgentToolID, "profile": agentID, "enabled": true})
	_, err := c.do(ctx, http.MethodPost, "/integrations-vbai/workspaces/"+w.ID+"/tools", body, apiTimeout)
	return err
}

// RemoveAgent removes the agent from the project. A missing row is not an
// error: it may have been deleted in the web interface.
func (c *Client) RemoveAgent(ctx context.Context, w Workspace, agentID string) error {
	t, ok := FindAgentTool(w, agentID)
	if !ok {
		return nil
	}
	_, err := c.do(ctx, http.MethodDelete,
		fmt.Sprintf("/integrations-vbai/workspaces/%s/tools/%d", w.ID, t.ID), nil, apiTimeout)
	return err
}

// SetToolEnabled flips a tool row on or off — the same switch as in the web
// interface, since the row is the same.
func (c *Client) SetToolEnabled(ctx context.Context, workspaceID string, rowID int, enabled bool) error {
	body, _ := json.Marshal(map[string]any{"enabled": enabled})
	_, err := c.do(ctx, http.MethodPut,
		fmt.Sprintf("/integrations-vbai/workspaces/%s/tools/%d", workspaceID, rowID), body, apiTimeout)
	return err
}

// CreateWorkspace makes a new project in integrations-vbai (/project publish)
// and returns it: the object itself, or unwrapped from {"workspace": …}.
func (c *Client) CreateWorkspace(ctx context.Context, name string) (Workspace, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	data, err := c.do(ctx, http.MethodPost, "/integrations-vbai/workspaces", body, apiTimeout)
	if err != nil {
		return Workspace{}, err
	}
	var ws Workspace
	if err := json.Unmarshal(data, &ws); err == nil && ws.ID != "" {
		return ws, nil
	}
	var wrapped struct {
		Workspace Workspace `json:"workspace"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Workspace.ID != "" {
		return wrapped.Workspace, nil
	}
	return Workspace{}, fmt.Errorf("%s: %s", i18n.T("inbox.err.parse"), strings.TrimSpace(string(data)))
}
