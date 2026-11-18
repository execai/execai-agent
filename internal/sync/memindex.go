package sync

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The memory index MEMORY.md is merged entry by entry, not decided as one
// file: the agent sees a note only through its line, so in the merged index
// every note that exists has a line, a line goes only together with its
// note, and the text the person wrote around the lines is not lost. The
// rule, after the notes of this exchange are decided:
//
//   - the lines of the index here, on the server and of the base are joined
//     by the note they point at;
//   - for a note whose line differs: this side's when it changed it here
//     (here ≠ base), the server's otherwise; with no base known, the
//     server's;
//   - a line whose note was deleted (here, on the server or on another
//     computer) goes; a line whose note sync never saw stays (the model may
//     write the line before the note, and a note another computer cannot
//     send — a link, too big — is on no server);
//   - a note that exists while both sides dropped its line gets it back
//     from the base; a note sync knew before this exchange that has no line
//     at all gets one, made from the description in its header, or from its
//     name;
//   - every other line (headings, paragraphs, comments) is the file's text
//     and is decided by the table (threeway.go) on its own, compared
//     without the note lines: changed here only — this side's text goes up;
//     changed on the server only — the server's comes here; changed on both
//     sides, or nothing to compare with — the server's stays.
//
// What the index here held and the merge does not carry is kept before the
// index is replaced: in the history when all of it was agreed with the
// server (here = base, row 2a), as a permanent copy, and said, when a line
// of it never reached the server (rows 4 and 5c). An index without a single
// entry (the template) never goes up — with a base or without one.

// entryRe — a line that points at a note: a markdown link to a .md file.
var entryRe = regexp.MustCompile(`\]\(<?([^)<>\s]+\.md)>?\)`)

type indexLine struct {
	text string
	note string // the note the line points at; "" for any other line
	// blank — nothing to read in it: an empty line, or one inside an HTML
	// comment only.
	blank bool
}

// parseIndex splits an index into its lines; lines inside an HTML comment
// are never note lines.
func parseIndex(data []byte) []indexLine {
	if len(data) == 0 {
		return nil
	}
	var out []indexLine
	in := false
	for _, raw := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		t := strings.TrimRight(raw, " \t\r")
		visible := ""
		rest := t
		for rest != "" {
			if in {
				i := strings.Index(rest, "-->")
				if i < 0 {
					rest = ""
					break
				}
				in = false
				rest = rest[i+3:]
				continue
			}
			i := strings.Index(rest, "<!--")
			if i < 0 {
				visible += rest
				break
			}
			visible += rest[:i]
			in = true
			rest = rest[i+4:]
		}
		l := indexLine{text: t, blank: strings.TrimSpace(visible) == ""}
		if m := entryRe.FindStringSubmatch(visible); m != nil {
			if n := noteName(m[1]); n != "" {
				l.note = n
			}
		}
		out = append(out, l)
	}
	return out
}

// noteName — the note a link points at, as a path in the memory folder.
func noteName(link string) string {
	if strings.Contains(link, "://") || strings.HasPrefix(link, "/") {
		return ""
	}
	n := path.Clean(strings.TrimPrefix(link, "./"))
	if !safeRel(n) {
		return ""
	}
	return n
}

// noteLines — note → its line in lines: the first one, or with several
// lines for one note the first that is not in prefer-not (a line the base
// had loses to one written since).
func noteLines(lines []indexLine, old map[string]bool) (map[string]string, []string) {
	m := map[string]string{}
	var order []string
	for _, l := range lines {
		if l.note == "" {
			continue
		}
		cur, ok := m[l.note]
		switch {
		case !ok:
			m[l.note] = l.text
			order = append(order, l.note)
		case old[cur] && !old[l.text]:
			m[l.note] = l.text
		}
	}
	return m, order
}

// freeText — the lines of an index that are not note lines and say
// something, as one text: what the table compares for them.
func freeText(lines []indexLine) string {
	var b strings.Builder
	for _, l := range lines {
		if l.note == "" && !l.blank {
			b.WriteString(l.text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// mergeIndex — the merged index (see the top of the file). extra — lines
// made for notes that may have none (added where the merge gives none).
// unsent: a line of the index here that the merge does not carry and the
// server never had (not in the base).
func mergeIndex(l []byte, lHere bool, s []byte, sHere bool, b []byte, bKnown bool,
	exists, deleted func(string) bool, extra []indexLine) (out []byte, unsent bool) {
	L, S, Bl := parseIndex(l), parseIndex(s), parseIndex(b)
	if !lHere {
		L = nil
	}
	if !sHere {
		S = nil
	}
	inBase := map[string]bool{}
	if bKnown {
		for _, x := range Bl {
			inBase[x.text] = true
		}
	}
	// Which side's text around the note lines stays: the table on that
	// text alone; the note lines are merged below.
	fl, fs, fb := freeText(L), freeText(S), freeText(Bl)
	var P []indexLine
	switch {
	case !sHere:
		P = L
	case !lHere:
		P = S
	case fl == fs:
		switch {
		case bytes.Equal(l, s), bKnown && bytes.Equal(s, b):
			P = L
		default:
			P = S
		}
	case bKnown && fl == fb:
		P = S // changed on the server only
	case bKnown && fs == fb:
		P = L // changed here only
	default:
		P = S // both changed, or nothing to compare: the server's
	}
	ll, lo := noteLines(L, inBase)
	sl, so := noteLines(S, nil)
	bl, bo := noteLines(Bl, nil)
	var notes []string
	seen := map[string]bool{}
	for _, list := range [][]string{lo, so, bo} {
		for _, n := range list {
			if !seen[n] {
				seen[n] = true
				notes = append(notes, n)
			}
		}
	}
	chosen := map[string]string{}
	for _, n := range notes {
		if deleted(n) {
			continue
		}
		lt, lok := ll[n]
		st, sok := sl[n]
		bt, bok := bl[n]
		switch {
		case bKnown && lok && (!bok || lt != bt):
			chosen[n] = lt // changed or added here
		case sok:
			chosen[n] = st
		case lok:
			chosen[n] = lt
		case bok && exists(n):
			chosen[n] = bt // the note is here, both sides dropped its line
		}
	}
	var lines []string
	done := map[string]bool{}
	for _, x := range P {
		if x.note == "" {
			lines = append(lines, x.text)
			continue
		}
		if t, ok := chosen[x.note]; ok && !done[x.note] {
			lines = append(lines, t)
			done[x.note] = true
		}
	}
	for _, n := range notes {
		if t, ok := chosen[n]; ok && !done[n] {
			lines = append(lines, t)
			done[n] = true
		}
	}
	for _, x := range extra {
		if x.note != "" && !done[x.note] && !deleted(x.note) && exists(x.note) {
			lines = append(lines, x.text)
			done[x.note] = true
		}
	}
	if len(lines) > 0 {
		out = []byte(strings.Join(lines, "\n") + "\n")
	}
	have := map[string]bool{}
	for _, t := range lines {
		have[t] = true
	}
	for _, x := range L {
		switch {
		case x.blank, have[x.text], inBase[x.text]:
		case x.note != "" && deleted(x.note):
		default:
			unsent = true
		}
	}
	return out, unsent
}

// headerLine — the line an index gets for the note rel that has none: its
// title and description from the header (--- name: … description: … ---),
// or its file name. "" when no line can point at it (a name a link cannot
// hold).
func headerLine(rel string, data []byte) string {
	title := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	desc := ""
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if strings.HasPrefix(text, "---\n") {
		if end := strings.Index(text[4:], "\n---"); end >= 0 {
			for _, ln := range strings.Split(text[4:4+end], "\n") {
				k, v, ok := strings.Cut(ln, ":")
				if !ok || strings.HasPrefix(ln, " ") || strings.HasPrefix(ln, "\t") {
					continue
				}
				v = strings.Trim(strings.TrimSpace(v), `"'`)
				switch strings.TrimSpace(k) {
				case "description":
					desc = v
				}
			}
		}
	}
	title = strings.NewReplacer("[", "", "]", "", "\n", " ").Replace(title)
	line := "- [" + title + "](" + rel + ")"
	if desc = strings.ReplaceAll(desc, "\n", " "); desc != "" {
		line += " — " + desc
	}
	if p := parseIndex([]byte(line)); len(p) != 1 || p[0].note != rel {
		return ""
	}
	return line
}

// syncMemoryIndex merges and exchanges MEMORY.md after the notes. known —
// the notes (paths in the memory folder) sync knew before this exchange
// and found here: one of them without a line gets one in a full exchange (a
// note just written may get its line from the model in a moment).
func (e *Engine) syncMemoryIndex(ctx context.Context, st *State, bs *Bases, remote map[string]Meta, idx string, known []string, rep *Report) error {
	memDir := e.Paths.MemoryDir()
	pl := place{kind: KindMemory, root: memDir, rel: "MEMORY.md"}
	_, name := splitKey(idx)
	k := e.keeper(rep)
	exists := func(n string) bool {
		fi, err := os.Lstat(filepath.Join(memDir, filepath.FromSlash(n)))
		return err == nil && !fi.IsDir()
	}
	deleted := func(n string) bool {
		if exists(n) {
			return false
		}
		key := blobKey(KindMemory, memoryName(e.ID, n))
		if b, ok := st.Blobs[key]; ok {
			return b.Deleted
		}
		m, ok := remote[key]
		return ok && m.Deleted
	}
	for attempt := 0; attempt < 3; attempt++ {
		b, had := st.Blobs[idx]
		lData, lHere, err := k.read(pl)
		if err != nil {
			return nil // a link, or cannot be read: left out (local.go said so)
		}
		lFile := lHere
		if lHere && len(indexEntries(lData)) == 0 {
			lHere = false // the template, or an empty file: not an index
		}
		baseText := []byte(bs.MemoryIndex)
		baseKnown := had && !b.Deleted && len(baseText) > 0 && hashOf(baseText) == b.Hash
		var sData []byte
		sHere := false
		sVer := int64(0)
		switch {
		case remote == nil:
			// Send-only: the server is taken for the base — when the base's
			// lines are known; otherwise the next full exchange merges.
			if !baseKnown {
				return nil
			}
			sData, sHere, sVer = baseText, true, b.Version
		default:
			m, listed := remote[idx]
			switch {
			case attempt == 0 && !listed:
			case attempt == 0 && m.Deleted:
				sVer = m.Version
			case attempt == 0 && had && m.Version == b.Version && baseKnown:
				sData, sHere, sVer = baseText, true, b.Version
			default:
				f, err := e.fetch(ctx, KindMemory, name, m.Version)
				if err != nil {
					return err
				}
				switch {
				case f.hidden:
					rep.hide(idx)
					return nil
				case f.none:
					return nil // listed but not given: next time
				case f.gone:
					sVer = f.ver
				default:
					sData, sHere, sVer = f.data, true, f.ver
				}
			}
		}
		if sHere && len(indexEntries(sData)) == 0 {
			sHere = false
		}
		var bData []byte
		bKnown := false
		switch {
		case baseKnown:
			bData, bKnown = baseText, true
		case had && !b.Deleted && lFile && hashOf(lData) == b.Hash:
			bData, bKnown = lData, true
		case had && !b.Deleted && sHere && hashOf(sData) == b.Hash:
			bData, bKnown = sData, true
		case had && b.Deleted:
			bKnown = true
		}
		var extra []indexLine
		if remote != nil { // with no index anywhere, one is made for the notes
			for _, n := range known {
				fi, err := os.Lstat(filepath.Join(memDir, filepath.FromSlash(n)))
				if err != nil || !fi.Mode().IsRegular() {
					continue
				}
				data, err := os.ReadFile(filepath.Join(memDir, filepath.FromSlash(n)))
				if err != nil {
					continue
				}
				if line := headerLine(n, data); line != "" {
					extra = append(extra, indexLine{text: line, note: n})
				}
			}
		}
		merged, unsent := mergeIndex(lData, lHere, sData, sHere, bData, bKnown, exists, deleted, extra)
		// What the index here held and the merge does not carry: in the
		// history when the server had all of it, kept for good and said when
		// a line never reached it.
		keep, why := keepHistory, reasonNone
		if unsent {
			keep, why = keepUnsent, reasonIndex
		}
		if len(indexEntries(merged)) == 0 {
			// An index without a single entry never goes up. When the one
			// here has only lines of notes deleted everywhere (all of them
			// deleted at once), those lines go here all the same.
			if remote != nil && lFile && len(indexEntries(lData)) > 0 && !bytes.Equal(merged, lData) {
				if _, err := k.put(pl, merged, time.Time{}, hashOf(lData), keep, why, false); err != nil {
					switch {
					case isChanged(err):
						rep.Changed = addArea(rep.Changed, KindMemory, memDir, pl.file())
						return nil
					case isLocalErr(err):
						rep.Skipped = append(rep.Skipped, Skip{Path: pl.file(), Reason: SkipUnreadable, Cause: causeWrite})
						return nil
					}
					return err
				}
				rep.pulled(KindMemory)
			}
			return nil
		}
		if !lFile || !bytes.Equal(merged, lData) {
			expect := ""
			if lFile {
				expect = hashOf(lData)
			}
			if _, err := k.put(pl, merged, time.Time{}, expect, keep, why, false); err != nil {
				switch {
				case isChanged(err):
					rep.Changed = addArea(rep.Changed, KindMemory, memDir, pl.file())
					return nil
				case isLocalErr(err):
					rep.Skipped = append(rep.Skipped, Skip{Path: pl.file(), Reason: SkipUnreadable, Cause: causeWrite})
					return nil
				}
				return err
			}
			if !lFile && had && !b.Deleted {
				rep.Restored = addArea(rep.Restored, KindMemory, memDir, pl.file())
			}
			if unsent && bKnown {
				rep.Conflicts++
			}
			rep.pulled(KindMemory)
		}
		if !sHere || !bytes.Equal(merged, sData) {
			v, ok, err := e.upload(ctx, KindMemory, name, "MEMORY.md", pl.file(), merged, sVer, rep)
			var ce *ConflictError
			if errors.As(err, &ce) {
				if remote == nil {
					return nil // send-only: the next full exchange merges again
				}
				continue
			}
			if err != nil || !ok {
				return err
			}
			sVer = v
		}
		setMemBase(st, bs, idx, BlobState{Path: "MEMORY.md", Version: sVer, Hash: hashOf(merged)})
		bs.MemoryIndex = string(merged)
		return nil
	}
	return nil
}
