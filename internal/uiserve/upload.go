package uiserve

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// maxUploadBytes — one attached file; the same ceiling as documents in the
// web chat. A variable for tests.
var maxUploadBytes int64 = 25 << 20

// uploadsRel — where attached files go, relative to the project. Project
// sync keeps this folder local (internal/sync UploadsDirName).
var uploadsRel = filepath.Join(".execai", "uploads")

var (
	errTooLarge   = errors.New("file too large")
	errUploadsDir = errors.New("the uploads folder is not inside the project")
)

// handleUpload saves the first file of a multipart form and answers
// {path, name}: the absolute path to attach (as the editor panel does) and
// the saved file name.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authed(r) || !s.originOK(r, true) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "multipart/form-data" {
		http.Error(w, "want multipart/form-data", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			http.Error(w, "no file in the form", http.StatusBadRequest)
			return
		}
		if err != nil {
			uploadError(w, err)
			return
		}
		if part.FileName() == "" {
			_ = part.Close()
			continue
		}
		abs, name, err := s.save(part.FileName(), part)
		_ = part.Close()
		if err != nil {
			uploadError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"path": abs, "name": name})
		return
	}
}

func uploadError(w http.ResponseWriter, err error) {
	var mbe *http.MaxBytesError
	switch {
	case errors.Is(err, errTooLarge), errors.As(err, &mbe):
		http.Error(w, errTooLarge.Error(), http.StatusRequestEntityTooLarge)
	case errors.Is(err, errUploadsDir):
		http.Error(w, err.Error(), http.StatusForbidden)
	default:
		http.Error(w, "upload failed", http.StatusBadRequest)
	}
}

// save writes src to <cwd>/.execai/uploads/<time>-<safe name>. The name the
// browser sent is reduced to letters, digits, dot, dash and underscore — no
// directories, no hidden files; a file is never overwritten, and a symlinked
// uploads folder pointing out of the project is refused.
func (s *Server) save(orig string, src io.Reader) (abs, name string, err error) {
	dir, err := s.uploadsDir()
	if err != nil {
		return "", "", err
	}
	base := safeName(orig)
	stamp := time.Now().UnixMilli()
	for i := 0; i < 100; i++ {
		name = fmt.Sprintf("%d-%s", stamp, base)
		if i > 0 {
			name = fmt.Sprintf("%d-%d-%s", stamp, i, base)
		}
		file := filepath.Join(dir, name)
		// O_EXCL: an existing file — or a symlink planted under this name —
		// is never written through.
		f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		n, err := io.Copy(f, io.LimitReader(src, maxUploadBytes+1))
		cerr := f.Close()
		if err == nil && n > maxUploadBytes {
			err = errTooLarge
		}
		if err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(file)
			return "", "", err
		}
		return filepath.Join(s.opts.Cwd, uploadsRel, name), name, nil
	}
	return "", "", fs.ErrExist
}

// uploadsDir creates the uploads folder and returns its real path, which must
// be exactly <real cwd>/.execai/uploads.
func (s *Server) uploadsDir() (string, error) {
	if s.opts.Cwd == "" {
		return "", errUploadsDir
	}
	root, err := filepath.EvalSymlinks(s.opts.Cwd)
	if err != nil {
		return "", err
	}
	want := filepath.Join(root, uploadsRel)
	if err := os.MkdirAll(filepath.Join(s.opts.Cwd, uploadsRel), 0o700); err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(filepath.Join(s.opts.Cwd, uploadsRel))
	if err != nil {
		return "", err
	}
	if real != want {
		return "", errUploadsDir
	}
	return real, nil
}

// safeName — the base of a browser-supplied file name, safe to create in one
// folder on any system.
func safeName(orig string) string {
	orig = path.Base(strings.ReplaceAll(orig, "\\", "/"))
	var b strings.Builder
	for _, r := range orig {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	n := strings.Trim(b.String(), "._")
	if rs := []rune(n); len(rs) > 100 {
		ext := path.Ext(n)
		if len([]rune(ext)) > 16 {
			ext = ""
		}
		n = string(rs[:100-len([]rune(ext))]) + ext
	}
	if n == "" {
		return "file"
	}
	return n
}
