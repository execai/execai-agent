package brand

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestIcon(t *testing.T) {
	cfg, err := png.DecodeConfig(bytes.NewReader(IconPNG))
	if err != nil || cfg.Width != IconSize || cfg.Height != IconSize {
		t.Fatalf("icon.png: %v %dx%d", err, cfg.Width, cfg.Height)
	}
	ico := IconICO()
	if binary.LittleEndian.Uint16(ico[2:]) != 1 || binary.LittleEndian.Uint16(ico[4:]) != 1 ||
		int(binary.LittleEndian.Uint32(ico[14:])) != len(IconPNG) || !bytes.Equal(ico[22:], IconPNG) {
		t.Error(".ico does not carry the PNG as its one image")
	}
	icns := IconICNS()
	if string(icns[:4]) != "icns" || int(binary.BigEndian.Uint32(icns[4:])) != len(icns) ||
		string(icns[8:12]) != "ic08" || int(binary.BigEndian.Uint32(icns[12:])) != 8+len(IconPNG) ||
		!bytes.Equal(icns[16:], IconPNG) {
		t.Error(".icns does not carry the PNG as ic08")
	}
}
