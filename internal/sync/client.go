package sync

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/version"
)

// apiPrefix — the sync API behind the gateway.
const apiPrefix = "/agents-vbai/sync"

// RequestTimeout bounds one request to the sync API.
const RequestTimeout = 30 * time.Second

// Errors of the sync API.
var (
	ErrNotFound     = errors.New("sync: not found")
	ErrGone         = errors.New("sync: deleted on the server")
	ErrTooLarge     = errors.New("sync: file is over the size limit")
	ErrQuota        = errors.New("sync: storage quota is used up")
	ErrUnauthorized = errors.New("sync: token rejected")
)

// ConflictError — 409: the server holds another version than expected.
// Version 0 with a zero UpdatedAt: there is no such record at all (after
// «delete everything»). Deleted: the record is a tombstone — writing with
// its Version brings the file back.
type ConflictError struct {
	Version   int64
	UpdatedAt time.Time
	Deleted   bool
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("sync: version conflict (server has %d)", e.Version)
}

// StatusError — any other unexpected HTTP status.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string { return fmt.Sprintf("sync: HTTP %d: %s", e.Code, e.Body) }

// Time reads the server's timestamps. agents-vbai sends RFC 3339 UTC with
// milliseconds; unix seconds (or milliseconds) are read too, and null is
// the zero time (a 409 for a record that does not exist).
type Time struct{ time.Time }

// UnmarshalJSON accepts a number, a numeric string or an RFC 3339 string.
func (t *Time) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	t.Time = parseServerTime(s)
	return nil
}

// MarshalJSON writes unix seconds.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("0"), nil
	}
	return []byte(strconv.FormatInt(t.Unix(), 10)), nil
}

func parseServerTime(s string) time.Time {
	if s == "" || s == "null" || s == "0" {
		return time.Time{}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1e12 {
			return time.UnixMilli(n)
		}
		return time.Unix(n, 0)
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}

// Meta — one blob as the server lists it (tombstones included).
type Meta struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Version   int64  `json:"version"`
	Size      int64  `json:"size"`
	UpdatedAt Time   `json:"updated_at"`
	Deleted   bool   `json:"deleted"`
}

// Machine — one machine of the person, as the server lists it. Its name
// comes from the auth session, not from the manifest.
type Machine struct {
	AgentID      string   `json:"agent_id"`
	Sources      []string `json:"sources"`
	Active       string   `json:"active"`
	AgentVersion string   `json:"agent_version"`
	SyncedAt     Time     `json:"synced_at"`
}

// ServerState — GET /sync/state.
type ServerState struct {
	Identity *struct {
		Present   bool  `json:"present"`
		Version   int64 `json:"version"`
		UpdatedAt Time  `json:"updated_at"`
	} `json:"identity"`
	Blobs    []Meta    `json:"blobs"`
	Machines []Machine `json:"machines"`
	Quota    struct {
		UsedBytes  int64 `json:"used_bytes"`
		LimitBytes int64 `json:"limit_bytes"`
		Blobs      int   `json:"blobs"`
		LimitBlobs int   `json:"limit_blobs"`
	} `json:"quota"`
}

// RemoteIdentity — the wrapped key as the server keeps it.
type RemoteIdentity struct {
	Wrapped   []byte
	Version   int64
	UpdatedAt time.Time
}

// Client talks to /agents-vbai/sync/*.
type Client struct {
	Base string
	// Token returns the current ExecAI JWT; read on every request, a login
	// in the chat replaces credentials.json mid-process.
	Token func() string
	HTTP  *http.Client
}

// NewClient builds a client for the API base from config.
func NewClient(apiBase string, token func() string) *Client {
	return &Client{
		Base:  strings.TrimRight(apiBase, "/"),
		Token: token,
		HTTP:  &http.Client{Timeout: RequestTimeout},
	}
}

// do performs one request; a status >= 400 becomes one of the errors above.
func (c *Client) do(ctx context.Context, method, path string, body []byte, hdr map[string]string, limit int64) (*http.Response, []byte, error) {
	token := ""
	if c.Token != nil {
		token = c.Token()
	}
	if token == "" {
		return nil, nil, ErrUnauthorized
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", version.UserAgent())
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, nil, err
	}
	switch code := resp.StatusCode; {
	case code < 300:
		return resp, data, nil
	case code == http.StatusUnauthorized:
		return resp, nil, ErrUnauthorized
	case code == http.StatusNotFound:
		return resp, nil, ErrNotFound
	case code == http.StatusGone:
		return resp, nil, ErrGone
	case code == http.StatusRequestEntityTooLarge:
		return resp, nil, ErrTooLarge
	case code == http.StatusInsufficientStorage:
		return resp, nil, ErrQuota
	case code == http.StatusConflict:
		var cv struct {
			Version   int64 `json:"version"`
			UpdatedAt Time  `json:"updated_at"`
			Deleted   bool  `json:"deleted"`
		}
		_ = json.Unmarshal(data, &cv)
		return resp, nil, &ConflictError{Version: cv.Version, UpdatedAt: cv.UpdatedAt.Time, Deleted: cv.Deleted}
	default:
		b := strings.TrimSpace(string(data))
		if len(b) > 200 {
			b = b[:200]
		}
		return resp, nil, &StatusError{Code: code, Body: b}
	}
}

const jsonLimit = 4 << 20

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	_, data, err := c.do(ctx, http.MethodGet, path, nil, nil, jsonLimit)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("sync: bad answer from %s: %w", path, err)
	}
	return nil
}

func (c *Client) sendJSON(ctx context.Context, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	_, data, err := c.do(ctx, method, path, body, map[string]string{"Content-Type": "application/json"}, jsonLimit)
	if err != nil {
		return err
	}
	if out != nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("sync: bad answer from %s: %w", path, err)
		}
	}
	return nil
}

// State — GET /sync/state.
func (c *Client) State(ctx context.Context) (*ServerState, error) {
	var st ServerState
	if err := c.getJSON(ctx, apiPrefix+"/state", &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// GetIdentity — GET /sync/identity; ErrNotFound when there is none.
func (c *Client) GetIdentity(ctx context.Context) (*RemoteIdentity, error) {
	var r struct {
		Wrapped   string `json:"wrapped"`
		Version   int64  `json:"version"`
		UpdatedAt Time   `json:"updated_at"`
	}
	if err := c.getJSON(ctx, apiPrefix+"/identity", &r); err != nil {
		return nil, err
	}
	w, err := base64.StdEncoding.DecodeString(r.Wrapped)
	if err != nil || len(w) == 0 {
		return nil, fmt.Errorf("sync: bad wrapped key from the server")
	}
	return &RemoteIdentity{Wrapped: w, Version: r.Version, UpdatedAt: r.UpdatedAt.Time}, nil
}

// PutIdentity — PUT /sync/identity. expected: the version being replaced,
// 0 for a new one. Returns the new version, or *ConflictError.
func (c *Client) PutIdentity(ctx context.Context, wrapped []byte, expected int64) (int64, error) {
	in := map[string]any{"wrapped": base64.StdEncoding.EncodeToString(wrapped), "version": expected}
	var out struct {
		Version int64 `json:"version"`
	}
	if err := c.sendJSON(ctx, http.MethodPut, apiPrefix+"/identity", in, &out); err != nil {
		return 0, err
	}
	return out.Version, nil
}

// List — GET /sync/blobs?since=, tombstones included. since is unix
// seconds, inclusive; 0 lists everything. The engine always lists
// everything (see Engine.pull), so next_since of the answer is not used.
func (c *Client) List(ctx context.Context, since int64) ([]Meta, error) {
	var r struct {
		Blobs []Meta `json:"blobs"`
	}
	if err := c.getJSON(ctx, apiPrefix+"/blobs?since="+strconv.FormatInt(since, 10), &r); err != nil {
		return nil, err
	}
	return r.Blobs, nil
}

func blobPath(kind, name string) string {
	return apiPrefix + "/blobs/" + url.PathEscape(kind) + "/" + url.PathEscape(name)
}

// Get — GET /sync/blobs/{kind}/{name}: the ciphertext, its version and when
// it was written. ErrNotFound, or ErrGone for a tombstone.
func (c *Client) Get(ctx context.Context, kind, name string) ([]byte, int64, time.Time, error) {
	resp, data, err := c.do(ctx, http.MethodGet, blobPath(kind, name), nil, nil, MaxProjectBlobBytes+64<<10)
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	v, _ := strconv.ParseInt(resp.Header.Get("X-Sync-Version"), 10, 64)
	return data, v, parseServerTime(resp.Header.Get("X-Sync-Updated-At")), nil
}

// Put — PUT /sync/blobs/{kind}/{name}. expected: the version being
// replaced, 0 for a new blob. Returns the new version, or *ConflictError,
// ErrTooLarge, ErrQuota.
func (c *Client) Put(ctx context.Context, kind, name string, body []byte, expected int64) (int64, error) {
	hdr := map[string]string{
		"Content-Type":   "application/octet-stream",
		"X-Sync-Version": strconv.FormatInt(expected, 10),
	}
	_, data, err := c.do(ctx, http.MethodPut, blobPath(kind, name), body, hdr, jsonLimit)
	if err != nil {
		return 0, err
	}
	var out struct {
		Version int64 `json:"version"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return 0, fmt.Errorf("sync: bad answer to PUT: %w", err)
	}
	return out.Version, nil
}

// Delete — DELETE /sync/blobs/{kind}/{name}: leaves a tombstone. Returns
// the tombstone's version. expected > 0 — delete only that version: a file
// changed on another machine since then is not deleted blindly
// (*ConflictError). Deleting a tombstone again changes nothing.
func (c *Client) Delete(ctx context.Context, kind, name string, expected int64) (int64, error) {
	hdr := map[string]string{"Content-Type": "application/json"}
	if expected > 0 {
		hdr["X-Sync-Version"] = strconv.FormatInt(expected, 10)
	}
	_, data, err := c.do(ctx, http.MethodDelete, blobPath(kind, name), nil, hdr, jsonLimit)
	if err != nil {
		return 0, err
	}
	var out struct {
		Version int64 `json:"version"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return 0, fmt.Errorf("sync: bad answer to DELETE: %w", err)
	}
	return out.Version, nil
}

// PutMachine — PUT /sync/machines/{agent_id}: this machine's manifest.
func (c *Client) PutMachine(ctx context.Context, agentID string, m Manifest) error {
	return c.sendJSON(ctx, http.MethodPut, apiPrefix+"/machines/"+url.PathEscape(agentID), m, nil)
}

// DeleteMachine — DELETE /sync/machines/{agent_id}.
func (c *Client) DeleteMachine(ctx context.Context, agentID string) error {
	return c.sendJSON(ctx, http.MethodDelete, apiPrefix+"/machines/"+url.PathEscape(agentID), nil, nil)
}

// DeleteAll — DELETE /sync/all: the key, every file and every machine of
// the person. Returns how many files were deleted.
func (c *Client) DeleteAll(ctx context.Context) (int, error) {
	var out struct {
		DeletedBlobs int `json:"deleted_blobs"`
	}
	if err := c.sendJSON(ctx, http.MethodDelete, apiPrefix+"/all", nil, &out); err != nil {
		return 0, err
	}
	return out.DeletedBlobs, nil
}

// Binding — a project bound to a directory on one of the person's machines.
type Binding struct {
	AgentID     string `json:"agent_id"`
	WorkspaceID string `json:"workspace_id"`
	LocalPath   string `json:"local_path"`
}

// Bindings — GET /agents-vbai/workspaces/bindings (outside the sync prefix).
func (c *Client) Bindings(ctx context.Context) ([]Binding, error) {
	var r struct {
		Bindings []Binding `json:"bindings"`
	}
	if err := c.getJSON(ctx, "/agents-vbai/workspaces/bindings", &r); err != nil {
		return nil, err
	}
	return r.Bindings, nil
}

// Workspace — a project of the web console (integrations-vbai).
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Workspaces — GET /integrations-vbai/workspaces (a list, or {workspaces}).
func (c *Client) Workspaces(ctx context.Context) ([]Workspace, error) {
	_, data, err := c.do(ctx, http.MethodGet, "/integrations-vbai/workspaces", nil, nil, jsonLimit)
	if err != nil {
		return nil, err
	}
	var list []Workspace
	if json.Unmarshal(data, &list) == nil {
		return list, nil
	}
	var wrapped struct {
		Workspaces []Workspace `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("sync: bad project list: %w", err)
	}
	return wrapped.Workspaces, nil
}
