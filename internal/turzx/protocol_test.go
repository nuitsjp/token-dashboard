package turzx

import (
	"bytes"
	"crypto/cipher"
	"crypto/des"
	"encoding/binary"
	"testing"
	"time"
)

func TestRevAFullScreenBitmapCoordinates(t *testing.T) {
	for _, tc := range []struct {
		width, height int
		packet        []byte
	}{
		{480, 320, []byte{0, 0, 7, 125, 63, 197}},
		{320, 480, []byte{0, 0, 4, 253, 223, 197}},
	} {
		if got := revACommand(197, 0, 0, tc.width-1, tc.height-1); !bytes.Equal(got, tc.packet) {
			t.Fatalf("%dx%d bitmap header: % X", tc.width, tc.height, got)
		}
	}
}

func TestPacket(t *testing.T) {
	now := time.Date(2026, 9, 27, 1, 2, 3, 4_000_000, time.Local)
	p, timestamp := packet(cmdJPEG, []byte{1, 2, 3}, now)
	if len(p) != 515 || p[510] != 0xA1 || p[511] != 0x1A || p[512] != 1 {
		t.Fatalf("control part trailer or payload is wrong: % X", p[504:])
	}
	if want := uint32((3723*time.Second + 4*time.Millisecond).Milliseconds()); timestamp != want {
		t.Fatalf("timestamp %d, want %d", timestamp, want)
	}
	block, _ := des.NewCipher(desKey)
	header := make([]byte, 504)
	cipher.NewCBCDecrypter(block, desKey).CryptBlocks(header, p[:504])
	if header[0] != cmdJPEG || header[2] != 0x1A || header[3] != 0x6D ||
		binary.LittleEndian.Uint32(header[4:]) != timestamp || binary.BigEndian.Uint32(header[8:]) != 3 {
		t.Fatalf("header % X", header[:12])
	}
}

func TestCheckResponse(t *testing.T) {
	ok := []byte{cmdJPEG, 0xC8, 0x78, 0x56, 0x34, 0x12}
	if err := checkResponse(cmdJPEG, 0x12345678, ok); err != nil {
		t.Fatal(err)
	}
	if checkResponse(cmdJPEG, 0x12345679, ok) == nil || checkResponse(cmdSync, 0x12345678, ok) == nil || checkResponse(cmdJPEG, 0x12345678, ok[:5]) == nil {
		t.Fatal("a mismatching response was accepted")
	}
}
