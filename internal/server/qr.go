package server

import "rsc.io/qr"

// qrPNG renders text as a QR code no larger than size pixels (quiet zone
// included), so the plugin can show it at a predictable size on the TV.
func qrPNG(text string, size int) ([]byte, error) {
	if size < 100 || size > 1200 {
		size = 400
	}
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, err
	}
	// rsc.io/qr adds a 4-module border on each side.
	code.Scale = size / (code.Size + 8)
	if code.Scale < 1 {
		code.Scale = 1
	}
	return code.PNG(), nil
}
