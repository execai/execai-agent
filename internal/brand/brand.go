// Package brand holds the product icon the agent shows outside the terminal:
// the page of execai ui (favicon, web app manifest) and the launcher
// shortcuts. The picture is the one of the editor extension
// (vscode-vbai/media/icon.png), 256×256.
package brand

import (
	_ "embed"
	"encoding/binary"
)

// IconPNG — the icon, PNG 256×256 RGBA.
//
//go:embed icon.png
var IconPNG []byte

// IconSize — the side of IconPNG in pixels.
const IconSize = 256

// IconICO — the icon as a Windows .ico: one 256×256 entry that carries the
// PNG as it is (the form Windows reads since Vista).
func IconICO() []byte {
	out := make([]byte, 22, 22+len(IconPNG))
	binary.LittleEndian.PutUint16(out[2:], 1) // type: icon
	binary.LittleEndian.PutUint16(out[4:], 1) // one image
	// out[6], out[7] — width and height, 0 means 256; no palette, reserved.
	binary.LittleEndian.PutUint16(out[10:], 1)  // colour planes
	binary.LittleEndian.PutUint16(out[12:], 32) // bits per pixel
	binary.LittleEndian.PutUint32(out[14:], uint32(len(IconPNG)))
	binary.LittleEndian.PutUint32(out[18:], 22) // where the image starts
	return append(out, IconPNG...)
}

// IconICNS — the icon as a macOS .icns: one "ic08" entry (256×256), which
// carries the PNG as it is.
func IconICNS() []byte {
	out := make([]byte, 16, 16+len(IconPNG))
	copy(out, "icns")
	binary.BigEndian.PutUint32(out[4:], uint32(16+len(IconPNG)))
	copy(out[8:], "ic08")
	binary.BigEndian.PutUint32(out[12:], uint32(8+len(IconPNG)))
	return append(out, IconPNG...)
}
