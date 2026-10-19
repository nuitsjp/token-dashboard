package turzx

// Rev. A rotates the display's write direction, so RGB565 stays in upright row order.
func revAOrientation(width, height int, reverse180 bool) []byte {
	orientation := byte(100)
	if width > height {
		orientation = 102
	}
	if reverse180 {
		orientation++
	}
	command := make([]byte, 16)
	command[5], command[6] = 121, orientation
	command[7], command[8] = byte(width>>8), byte(width)
	command[9], command[10] = byte(height>>8), byte(height)
	return command
}

// Rev. A packs four 10-bit coordinates and a command into six bytes.
func revACommand(command byte, x, y, right, bottom int) []byte {
	return []byte{byte(x >> 2), byte((x&3)<<6 | y>>4), byte((y&15)<<4 | right>>6), byte((right&63)<<2 | bottom>>8), byte(bottom), command}
}
