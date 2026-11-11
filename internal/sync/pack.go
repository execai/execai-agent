package sync

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
)

// A pack — several files in one blob: a gzip-compressed JSON map
//
//	{"v":1,"files":{"<slash path>":"<base64 content>"}}
//
// The same files always give the same bytes (sorted keys, no gzip
// timestamp), so the hash of a pack says whether anything inside changed.
// Compressing before encrypting is safe here: nothing in a pack comes from
// anyone but the person.
type filePack struct {
	V     int               `json:"v"`
	Files map[string][]byte `json:"files"`
}

// maxUnpacked bounds what a pack may expand to.
const maxUnpacked = 32 << 20

var errPack = errors.New("sync: not a file pack")

func packFiles(files map[string][]byte) ([]byte, error) {
	raw, err := json.Marshal(filePack{V: 1, Files: files})
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	zw, err := gzip.NewWriterLevel(&b, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func unpackFiles(data []byte) (map[string][]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errPack
	}
	raw, err := io.ReadAll(io.LimitReader(zr, maxUnpacked+1))
	if err != nil || len(raw) > maxUnpacked {
		return nil, errPack
	}
	var p filePack
	if err := json.Unmarshal(raw, &p); err != nil || p.V != 1 {
		return nil, errPack
	}
	if p.Files == nil {
		p.Files = map[string][]byte{}
	}
	return p.Files, nil
}
