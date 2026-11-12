package sync

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	gosync "sync"
	"testing"
	"time"
)

// TestMain keeps the password hashing cheap: the real cost (2^18) would
// make every wrap a second and a quarter of a gigabyte.
func TestMain(m *testing.M) {
	ScryptWorkFactor = 10
	os.Exit(m.Run())
}

// stubServer is an in-memory /agents-vbai/sync/* that follows the contract
// of agents-vbai (handlers/sync.go): versions, tombstones, 409 bodies,
// mandatory X-Sync-Version on PUT, limits; plus /workspaces/bindings and
// /billing-vbai/my_plan for the gate.
type stubServer struct {
	*httptest.Server
	mu       gosync.Mutex
	clock    time.Time
	identity *stubIdentity
	blobs    map[string]*stubBlob
	machines map[string]map[string]any
	bindings []Binding
	// workspaces — the web projects, for their names.
	workspaces []Workspace
	// tariff answered by my_plan; "" — no subscription.
	tariff     string
	planCalls  int
	quotaBlobs int
	// listCalls — GET /sync/blobs requests so far; onList, when set, runs
	// before each of them is answered (outside mu), so a test can hold an
	// exchange in the middle.
	listCalls int
	onList    func()
	// failList — GET /sync/blobs answers 500.
	failList bool
	// onPut, when set, runs once before the next PUT of a blob is
	// answered (outside mu): another computer writing in between.
	onPut func(kind, name string)
	// keyFPs — the X-Sync-Key-FP of every write, in order;
	// identityRequired — writes without a key on the server are refused
	// (no_identity), as agents-vbai does.
	keyFPs           []string
	identityRequired bool
	// history — every version of every blob (remember).
	history map[string]map[int64]stubBlob
}

// remember keeps every version a blob had (for the tests' I1 guard: a base
// must be what the server held at its version). Caller holds mu.
func (s *stubServer) remember(b *stubBlob) {
	if s.history == nil {
		s.history = map[string]map[int64]stubBlob{}
	}
	k := b.kind + "/" + b.name
	if s.history[k] == nil {
		s.history[k] = map[int64]stubBlob{}
	}
	c := *b
	c.content = append([]byte(nil), b.content...)
	s.history[k][b.version] = c
}

// at — the blob as it was at version v.
func (s *stubServer) at(kind, name string, v int64) (stubBlob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.history[kind+"/"+name][v]
	return b, ok
}

// keyRefused answers as agents-vbai does for a write that names a key
// (X-Sync-Key-FP) other than the one on the server, or comes when the
// server has no key at all: 409 key_mismatch / no_identity.
func (s *stubServer) keyRefused(w http.ResponseWriter, r *http.Request) bool {
	fp := r.Header.Get("X-Sync-Key-FP")
	s.keyFPs = append(s.keyFPs, fp)
	if s.identity == nil {
		if s.identityRequired {
			writeJSONT(w, 409, map[string]any{"error": "no sync key", "code": "no_identity"})
			return true
		}
		return false // engine tests write without a key on the stub
	}
	if fp != "" && WrappedFingerprint(s.identity.wrapped) != "" && fp != WrappedFingerprint(s.identity.wrapped) {
		writeJSONT(w, 409, map[string]any{"error": "another key", "code": "key_mismatch"})
		return true
	}
	return false
}

// putDirect stores content as the next version of a blob, as another
// computer's PUT would.
func (s *stubServer) putDirect(kind, name string, content []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := kind + "/" + name
	b := s.blobs[k]
	if b == nil {
		b = &stubBlob{kind: kind, name: name}
		s.blobs[k] = b
	}
	b.content, b.version, b.updated, b.deleted = content, b.version+1, s.tick(), false
	s.remember(b)
}

type stubIdentity struct {
	wrapped []byte
	version int64
	updated time.Time
}

type stubBlob struct {
	kind, name string
	content    []byte
	version    int64
	updated    time.Time
	deleted    bool
}

func newStub(t *testing.T) *stubServer {
	s := &stubServer{
		clock:    time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
		blobs:    map[string]*stubBlob{},
		machines: map[string]map[string]any{},
		tariff:   "week_work",
	}
	mux := http.NewServeMux()
	p := "/agents-vbai/sync"
	mux.HandleFunc("GET "+p+"/state", s.state)
	mux.HandleFunc("GET "+p+"/identity", s.getIdentity)
	mux.HandleFunc("PUT "+p+"/identity", s.putIdentity)
	mux.HandleFunc("GET "+p+"/blobs", s.list)
	mux.HandleFunc("GET "+p+"/blobs/{kind}/{name}", s.getBlob)
	mux.HandleFunc("PUT "+p+"/blobs/{kind}/{name}", s.putBlob)
	mux.HandleFunc("DELETE "+p+"/blobs/{kind}/{name}", s.deleteBlob)
	mux.HandleFunc("PUT "+p+"/machines/{agent_id}", s.putMachine)
	mux.HandleFunc("DELETE "+p+"/machines/{agent_id}", s.deleteMachine)
	mux.HandleFunc("DELETE "+p+"/all", s.deleteAll)
	mux.HandleFunc("GET /agents-vbai/workspaces/bindings", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		writeJSONT(w, 200, map[string]any{"bindings": s.bindings})
	})
	mux.HandleFunc("GET /integrations-vbai/workspaces", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		writeJSONT(w, 200, s.workspaces)
	})
	mux.HandleFunc("GET /billing-vbai/my_plan", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.planCalls++
		if s.tariff == "" {
			writeJSONT(w, 200, map[string]any{"user_id": "u1", "subscription": nil, "is_accessible": false})
			return
		}
		writeJSONT(w, 200, map[string]any{"user_id": "u1", "tariff_code": s.tariff, "state": "active",
			"valid_until": "2099-01-01T00:00:00Z", "is_accessible": true})
	})
	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer tok-") {
				writeJSONT(w, 401, map[string]any{"error": "no"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	s.Server = httptest.NewServer(auth(mux))
	t.Cleanup(s.Close)
	return s
}

func writeJSONT(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// tick — every write happens a second later than the previous one.
func (s *stubServer) tick() time.Time {
	s.clock = s.clock.Add(time.Second)
	return s.clock
}

func ts(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z07:00") }

func (s *stubServer) conflict(w http.ResponseWriter, b *stubBlob) {
	if b == nil {
		writeJSONT(w, 409, map[string]any{"error": "conflict", "version": 0, "updated_at": nil})
		return
	}
	writeJSONT(w, 409, map[string]any{"error": "conflict", "version": b.version, "updated_at": ts(b.updated), "deleted": b.deleted})
}

func (s *stubServer) meta(b *stubBlob) map[string]any {
	return map[string]any{"kind": b.kind, "name": b.name, "version": b.version,
		"size": len(b.content), "updated_at": ts(b.updated), "deleted": b.deleted}
}

func (s *stubServer) state(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var id any
	if s.identity != nil {
		id = map[string]any{"present": true, "version": s.identity.version, "updated_at": ts(s.identity.updated)}
	}
	var metas []any
	for _, b := range s.blobs {
		metas = append(metas, s.meta(b))
	}
	var ms []any
	for agent, m := range s.machines {
		mm := map[string]any{"agent_id": agent}
		for k, v := range m {
			mm[k] = v
		}
		ms = append(ms, mm)
	}
	writeJSONT(w, 200, map[string]any{"identity": id, "blobs": metas, "machines": ms,
		"quota": map[string]any{"used_bytes": 0, "limit_bytes": 32 << 20, "blobs": len(s.blobs), "limit_blobs": 2000}})
}

func (s *stubServer) getIdentity(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.identity == nil {
		writeJSONT(w, 404, map[string]any{"error": "none"})
		return
	}
	writeJSONT(w, 200, map[string]any{"wrapped": s.identity.wrapped, "version": s.identity.version, "updated_at": ts(s.identity.updated)})
}

func (s *stubServer) putIdentity(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var req struct {
		Wrapped []byte `json:"wrapped"`
		Version *int64 `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Version == nil || len(req.Wrapped) == 0 {
		writeJSONT(w, 400, map[string]any{"error": "bad"})
		return
	}
	cur := int64(0)
	if s.identity != nil {
		cur = s.identity.version
	}
	if *req.Version != cur {
		if s.identity == nil {
			writeJSONT(w, 409, map[string]any{"version": 0, "updated_at": nil})
		} else {
			writeJSONT(w, 409, map[string]any{"version": cur, "updated_at": ts(s.identity.updated)})
		}
		return
	}
	s.identity = &stubIdentity{wrapped: req.Wrapped, version: cur + 1, updated: s.tick()}
	writeJSONT(w, 200, map[string]any{"version": cur + 1})
}

func (s *stubServer) list(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.listCalls++
	hook := s.onList
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failList {
		writeJSONT(w, 500, map[string]any{"error": "down"})
		return
	}
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	metas := []any{}
	for _, b := range s.blobs {
		if since > 0 && b.updated.Unix() < since {
			continue
		}
		metas = append(metas, s.meta(b))
	}
	writeJSONT(w, 200, map[string]any{"blobs": metas, "next_since": s.clock.Add(-30 * time.Second).Unix()})
}

func (s *stubServer) getBlob(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.blobs[r.PathValue("kind")+"/"+r.PathValue("name")]
	if b == nil {
		writeJSONT(w, 404, map[string]any{"error": "none"})
		return
	}
	w.Header().Set("X-Sync-Version", strconv.FormatInt(b.version, 10))
	w.Header().Set("X-Sync-Updated-At", ts(b.updated))
	if b.deleted {
		writeJSONT(w, 410, map[string]any{"error": "deleted", "version": b.version, "deleted": true})
		return
	}
	w.WriteHeader(200)
	_, _ = w.Write(b.content)
}

func (s *stubServer) putBlob(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	hook := s.onPut
	s.onPut = nil
	s.mu.Unlock()
	if hook != nil {
		hook(r.PathValue("kind"), r.PathValue("name"))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kind, name := r.PathValue("kind"), r.PathValue("name")
	if !knownKind(kind) || !validName(name) {
		writeJSONT(w, 400, map[string]any{"error": "bad name"})
		return
	}
	if s.keyRefused(w, r) {
		return
	}
	raw := r.Header.Get("X-Sync-Version")
	if raw == "" {
		writeJSONT(w, 400, map[string]any{"error": "X-Sync-Version required"})
		return
	}
	expected, _ := strconv.ParseInt(raw, 10, 64)
	body, _ := io.ReadAll(io.LimitReader(r.Body, int64(blobLimit(kind))+1))
	if len(body) > blobLimit(kind) {
		writeJSONT(w, 413, map[string]any{"error": "too large"})
		return
	}
	k := kind + "/" + name
	b := s.blobs[k]
	if s.quotaBlobs > 0 && (b == nil || b.deleted) && s.live() >= s.quotaBlobs {
		writeJSONT(w, 507, map[string]any{"error": "quota"})
		return
	}
	switch {
	case b == nil && expected == 0:
		b = &stubBlob{kind: kind, name: name}
		s.blobs[k] = b
	case b == nil || b.version != expected:
		s.conflict(w, b)
		return
	}
	b.content, b.version, b.updated, b.deleted = body, b.version+1, s.tick(), false
	s.remember(b)
	writeJSONT(w, 200, map[string]any{"version": b.version, "updated_at": ts(b.updated)})
}

func (s *stubServer) live() int {
	n := 0
	for _, b := range s.blobs {
		if !b.deleted {
			n++
		}
	}
	return n
}

func (s *stubServer) deleteBlob(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keyRefused(w, r) {
		return
	}
	b := s.blobs[r.PathValue("kind")+"/"+r.PathValue("name")]
	if b == nil {
		writeJSONT(w, 404, map[string]any{"error": "none"})
		return
	}
	if raw := r.Header.Get("X-Sync-Version"); raw != "" {
		if v, _ := strconv.ParseInt(raw, 10, 64); v != b.version {
			s.conflict(w, b)
			return
		}
	}
	if !b.deleted {
		b.content, b.version, b.updated, b.deleted = nil, b.version+1, s.tick(), true
		s.remember(b)
	}
	writeJSONT(w, 200, map[string]any{"version": b.version})
}

func (s *stubServer) putMachine(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var m map[string]any
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeJSONT(w, 400, map[string]any{"error": "bad"})
		return
	}
	s.machines[r.PathValue("agent_id")] = m
	writeJSONT(w, 200, map[string]any{"agent_id": r.PathValue("agent_id")})
}

func (s *stubServer) deleteMachine(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.machines, r.PathValue("agent_id"))
	writeJSONT(w, 200, map[string]any{"deleted": 1})
}

func (s *stubServer) deleteAll(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.live()
	s.blobs = map[string]*stubBlob{}
	s.machines = map[string]map[string]any{}
	had := s.identity != nil
	s.identity = nil
	writeJSONT(w, 200, map[string]any{"deleted_blobs": n, "deleted_machines": 0, "deleted_identity": had})
}

// blob returns a stored blob (nil if none).
func (s *stubServer) blob(kind, name string) *stubBlob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blobs[kind+"/"+name]
}

// allContent — every stored ciphertext, for the «no keys inside» checks.
func (s *stubServer) allContent() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out [][]byte
	for _, b := range s.blobs {
		out = append(out, b.content)
	}
	if s.identity != nil {
		out = append(out, s.identity.wrapped)
	}
	return out
}
