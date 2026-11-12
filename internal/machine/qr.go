package machine

import (
	"errors"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// QRLines draws a QR code for text with half blocks: one line of the
// terminal holds two rows of modules. Light modules are drawn as the
// foreground ("█"), dark ones as the background (" "), so the code reads
// on a dark terminal as is; the caller paints it white on black to make it
// read on any theme. The quiet zone is included.
func QRLines(text string) ([]string, error) {
	if text == "" {
		return nil, errors.New("empty text")
	}
	q, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	bm := q.Bitmap() // true = dark module
	light := func(y, x int) bool {
		if y >= len(bm) {
			return true // below the code: more quiet zone
		}
		return !bm[y][x]
	}
	var lines []string
	for y := 0; y < len(bm); y += 2 {
		var b strings.Builder
		for x := range bm[y] {
			top, bottom := light(y, x), light(y+1, x)
			switch {
			case top && bottom:
				b.WriteRune('█')
			case top:
				b.WriteRune('▀')
			case bottom:
				b.WriteRune('▄')
			default:
				b.WriteRune(' ')
			}
		}
		lines = append(lines, b.String())
	}
	return lines, nil
}
