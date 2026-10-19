package turzx

// Rev. A packs four 10-bit coordinates and a command into six bytes.
func revACommand(command byte, x, y, right, bottom int) []byte {
	return []byte{byte(x >> 2), byte((x&3)<<6 | y>>4), byte((y&15)<<4 | right>>6), byte((right&63)<<2 | bottom>>8), byte(bottom), command}
}
