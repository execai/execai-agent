package sync

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"os"
	"sort"
	"strings"
	"time"

	"filippo.io/age"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// With sync on, provider keys travel inside the config blob. keys export /
// import is the fallback without sync, without a plan and without our
// server: a transfer code — subscriptions.json encrypted with a fresh random
// phrase of six words — and the phrase separately; the person carries both
// over (a messenger, a USB stick, ideally by two different ways) and types
// them into keys import there.

// transferPrefix marks a transfer code.
const transferPrefix = "execai-keys-1:"

// PhraseWords — the number of words in a transfer phrase.
const PhraseWords = 6

var errBadCode = errors.New("sync: not a transfer code")

// transferPayload — what a transfer code carries.
type transferPayload struct {
	V             int                                   `json:"v"`
	Subscriptions map[string]subscriptions.Subscription `json:"subscriptions"`
}

// newPhrase picks n random words.
func newPhrase(n int) (string, error) {
	max := big.NewInt(int64(len(phraseWords)))
	out := make([]string, n)
	for i := range out {
		x, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = phraseWords[x.Int64()]
	}
	return strings.Join(out, " "), nil
}

// normalizePhrase forgives case, extra spaces and dashes between words.
func normalizePhrase(p string) string {
	p = strings.ToLower(strings.ReplaceAll(p, "-", " "))
	return strings.Join(strings.Fields(p), " ")
}

// sealTransfer encrypts the payload with the phrase into a transfer code.
func sealTransfer(payload []byte, phrase string) (string, error) {
	r, err := age.NewScryptRecipient(normalizePhrase(phrase))
	if err != nil {
		return "", err
	}
	r.SetWorkFactor(ScryptWorkFactor)
	var out bytes.Buffer
	w, err := age.Encrypt(&out, r)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(payload); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return transferPrefix + base64.RawURLEncoding.EncodeToString(out.Bytes()), nil
}

// parseTransferCode reads a pasted code: whitespace and line breaks a
// messenger may add are ignored.
func parseTransferCode(code string) ([]byte, error) {
	code = strings.Join(strings.Fields(code), "")
	if len(code) < len(transferPrefix) || !strings.EqualFold(code[:len(transferPrefix)], transferPrefix) {
		return nil, errBadCode
	}
	ct, err := base64.RawURLEncoding.DecodeString(code[len(transferPrefix):])
	if err != nil || len(ct) == 0 {
		return nil, errBadCode
	}
	return ct, nil
}

// openTransfer decrypts a transfer code with the phrase.
func openTransfer(ct []byte, phrase string) (*transferPayload, error) {
	id, err := age.NewScryptIdentity(normalizePhrase(phrase))
	if err != nil {
		return nil, ErrBadPassword
	}
	r, err := age.Decrypt(bytes.NewReader(ct), id)
	if err != nil {
		return nil, ErrBadPassword
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, ErrBadPassword
	}
	var p transferPayload
	if err := json.Unmarshal(data, &p); err != nil || p.V != 1 {
		return nil, errBadCode
	}
	return &p, nil
}

// KeysExport — /sync keys export: the transfer code and, separately, its
// phrase. No server is involved, so it needs neither a plan nor a login.
func (s *Service) KeysExport() string {
	store := s.Subs()
	if store == nil || len(store.Subscriptions) == 0 {
		return i18n.T("sync.keys.nothing")
	}
	p := transferPayload{V: 1, Subscriptions: map[string]subscriptions.Subscription{}}
	names := make([]string, 0, len(store.Subscriptions))
	for _, sub := range store.List() {
		// The model lists are fetched again on the other machine; without
		// them the code is a fraction of the size.
		sub.Models, sub.AvailableModels, sub.ModelsFetchedAt = nil, nil, time.Time{}
		p.Subscriptions[sub.Provider] = sub
		names = append(names, subscriptions.DisplayName(sub))
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return i18n.Tf("sync.error", err)
	}
	phrase, err := newPhrase(PhraseWords)
	if err != nil {
		return i18n.Tf("sync.error", err)
	}
	code, err := sealTransfer(payload, phrase)
	if err != nil {
		return i18n.Tf("sync.error", err)
	}
	return i18n.Tf("sync.keys.exported", strings.Join(names, ", "), code, s.Cmd) +
		"\n\n" + i18n.Tf("sync.keys.phrase", phrase)
}

// importKeys merges the code's sources into subscriptions.json. A source
// that is not here is added; one that is here with another key is replaced
// only if the imported one was connected later. The active source is never
// changed. subscriptions.json is part of the synced settings, so the
// merge runs under the exchange lock (errBusy while another process
// exchanges); opening the code — the slow part — does not.
func (s *Service) importKeys(ctx context.Context, ct []byte, phrase string) (string, error) {
	p, err := openTransfer(ct, phrase)
	if err != nil {
		return "", err
	}
	unlock, err := s.lock(ctx, s.commandWait())
	if err != nil {
		return "", err
	}
	defer unlock()
	store, err := subscriptions.Load()
	if err != nil {
		return "", err
	}
	if store.Subscriptions == nil {
		store.Subscriptions = map[string]subscriptions.Subscription{}
	}
	var added, updated, same []string
	ids := make([]string, 0, len(p.Subscriptions))
	for id := range p.Subscriptions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		sub := p.Subscriptions[id]
		if id == "" || id == subscriptions.SourceExecAI {
			continue
		}
		sub.Provider = id
		name := subscriptions.DisplayName(sub)
		cur, ok := store.Subscriptions[id]
		switch {
		case !ok:
			store.Subscriptions[id] = sub
			added = append(added, name)
		case sameSource(cur, sub):
			same = append(same, name)
		case sub.ConnectedAt.After(cur.ConnectedAt):
			store.Subscriptions[id] = sub
			updated = append(updated, name)
		default:
			same = append(same, name) // the one here is newer
		}
	}
	if len(added)+len(updated) > 0 {
		// Written through the keeper, as every change of a synced file: a
		// replaced key stays in a copy of the previous subscriptions.json.
		data, err := json.MarshalIndent(store, "", "  ")
		if err != nil {
			return "", err
		}
		expect := ""
		if raw, err := os.ReadFile(s.Paths.SubscriptionsFile()); err == nil {
			expect = hashOf(raw)
		}
		k := &keeper{paths: s.Paths, now: s.now, rep: &Report{}}
		pl := place{kind: KindConfig, root: s.Paths.ConfigDir, rel: "subscriptions.json"}
		if _, err := k.put(pl, data, time.Time{}, expect, keepUnsent, reasonNone, false); err != nil {
			return "", err
		}
	}
	var lines []string
	if len(added) > 0 {
		lines = append(lines, i18n.Tf("sync.keys.added", strings.Join(added, ", ")))
	}
	if len(updated) > 0 {
		lines = append(lines, i18n.Tf("sync.keys.updated", strings.Join(updated, ", ")))
	}
	if len(same) > 0 {
		lines = append(lines, i18n.Tf("sync.keys.kept", strings.Join(same, ", ")))
	}
	lines = append(lines, i18n.Tf("sync.keys.active", store.SourceLabel()))
	return strings.Join(lines, "\n"), nil
}

// sameSource — the same key and endpoint; model lists do not count.
func sameSource(a, b subscriptions.Subscription) bool {
	if a.APIKey != b.APIKey || a.BaseURL != b.BaseURL || a.API != b.API || a.Plan != b.Plan || len(a.Headers) != len(b.Headers) {
		return false
	}
	for k, v := range a.Headers {
		if b.Headers[k] != v {
			return false
		}
	}
	return true
}

// ===== /sync keys import =====

type keysImportFlow struct {
	s     *Service
	stage int
	code  string
	ct    []byte
}

// KeysImportFlow — /sync keys import [code]: the code (unless given on the
// command line), then the phrase.
func (s *Service) KeysImportFlow(code string) Flow {
	return &keysImportFlow{s: s, code: code}
}

func (f *keysImportFlow) Start(ctx context.Context) Step {
	if strings.TrimSpace(f.code) != "" {
		return f.Answer(ctx, f.code)
	}
	f.stage = stageCode
	return Step{Text: i18n.T("sync.keys.importIntro"), Prompt: i18n.T("sync.keys.enterCode"), Quiet: true}
}

func (f *keysImportFlow) Answer(ctx context.Context, a string) Step {
	switch f.stage {
	case stageCode, 0:
		ct, err := parseTransferCode(a)
		if err != nil {
			f.stage = stageCode
			return Step{Text: i18n.T("sync.keys.badCode"), Prompt: i18n.T("sync.keys.enterCode"), Quiet: true}
		}
		f.ct, f.stage = ct, stagePhrase
		return Step{Prompt: i18n.Tf("sync.keys.enterPhrase", PhraseWords), Quiet: true}
	case stagePhrase:
		text, err := f.s.importKeys(ctx, f.ct, a)
		switch {
		case errors.Is(err, ErrBadPassword):
			return Step{Text: i18n.T("sync.keys.badPhrase"), Prompt: i18n.Tf("sync.keys.enterPhrase", PhraseWords), Quiet: true}
		case errors.Is(err, errBadCode):
			return Step{Text: i18n.T("sync.keys.badCode")}
		case err != nil:
			return Step{Text: f.s.localErr(err)}
		}
		return Step{Text: text}
	}
	return Step{}
}
