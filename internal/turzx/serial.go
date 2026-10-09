package turzx

import (
	"encoding/binary"
	"fmt"
	"image"
)

// Rev. A uses a six-byte command: four ten-bit coordinates, then the command byte.
// Protocol reference: https://github.com/mathoudebine/turing-smart-screen-python/blob/main/library/lcd/lcd_comm_rev_a.py
func serialCommand(command byte, right, bottom int) []byte {
	coordinates := uint64(right)<<10 | uint64(bottom)
	data := make([]byte, 6)
	for i := 4; i >= 0; i-- {
		data[i] = byte(coordinates)
		coordinates >>= 8
	}
	data[5] = command
	return data
}

func serialOrientation(orientation string) ([]byte, int, int, error) {
	w, h, direction := 480, 320, byte(102)
	switch orientation {
	case "Landscape":
	case "ReverseLandscape":
		direction = 103
	case "Portrait":
		w, h, direction = 320, 480, 100
	case "ReversePortrait":
		w, h, direction = 320, 480, 101
	default:
		return nil, 0, 0, fmt.Errorf("unsupported compact orientation %q", orientation)
	}
	data := make([]byte, 16)
	data[5], data[6] = 121, direction
	binary.BigEndian.PutUint16(data[7:9], uint16(w))
	binary.BigEndian.PutUint16(data[9:11], uint16(h))
	return data, w, h, nil
}

// RGB565 is row-major with the least significant byte of each pixel first.
func RGB565(img *image.RGBA) []byte {
	b := img.Bounds()
	data := make([]byte, b.Dx()*b.Dy()*2)
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			p := img.RGBAAt(x, y)
			value := uint16(p.R>>3)<<11 | uint16(p.G>>2)<<5 | uint16(p.B>>3)
			binary.LittleEndian.PutUint16(data[i:i+2], value)
			i += 2
		}
	}
	return data
}
