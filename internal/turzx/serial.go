package turzx

import "image"

// rgb565Regions returns the smallest rectangles covering changed pixels in
// each run of consecutive rows.
func rgb565Regions(data, previous []byte, width, height int) []image.Rectangle {
	if previous == nil {
		return []image.Rectangle{image.Rect(0, 0, width, height)}
	}

	var regions []image.Rectangle
	inRegion := false
	regionMinX, regionMaxX, regionStartY := 0, 0, 0
	for y := 0; y < height; y++ {
		rowMinX, rowMaxX := width, -1
		rowOffset := y * width * 2
		for x := 0; x < width; x++ {
			offset := rowOffset + x*2
			if data[offset] == previous[offset] && data[offset+1] == previous[offset+1] {
				continue
			}
			if x < rowMinX {
				rowMinX = x
			}
			if x > rowMaxX {
				rowMaxX = x
			}
		}

		if rowMaxX == -1 {
			if inRegion {
				regions = append(regions, image.Rect(regionMinX, regionStartY, regionMaxX+1, y))
				inRegion = false
			}
			continue
		}

		if !inRegion {
			regionMinX, regionMaxX, regionStartY = rowMinX, rowMaxX, y
			inRegion = true
			continue
		}
		if rowMinX < regionMinX {
			regionMinX = rowMinX
		}
		if rowMaxX > regionMaxX {
			regionMaxX = rowMaxX
		}
	}
	if inRegion {
		regions = append(regions, image.Rect(regionMinX, regionStartY, regionMaxX+1, height))
	}
	return regions
}

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
