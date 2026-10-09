//go:build windows && resvgbench

// Command partial-resvg compares full SVG parsing with contract-region updates.
package main

/*
#include <stdlib.h>
#include <resvg.h>
*/
import "C"

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

const width, height = 1920, 462

type patch struct {
	SVG       string `json:"svg"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	LayoutKey string `json:"layoutKey"`
}

type timing struct {
	TemplateMS  float64 `json:"templateMs"`
	ParseMS     float64 `json:"parseMs"`
	RenderMS    float64 `json:"renderMs"`
	CompositeMS float64 `json:"compositeMs"`
	RotateMS    float64 `json:"rotateMs"`
	EncodeMS    float64 `json:"encodeMs"`
	TotalMS     float64 `json:"totalMs"`
	ObservedMS  float64 `json:"observedMs"`
	Bytes       int     `json:"bytes"`
}

type styleResult struct {
	Style          string           `json:"style"`
	SetupMS        float64          `json:"setupMs"`
	Region         image.Rectangle  `json:"region"`
	First          timing           `json:"first"`
	ReferenceRGBA  map[int]string   `json:"referenceRgbaHashes"`
	ReferenceImage map[int]string   `json:"referenceImageHashes"`
	ImageHashes    map[int][]string `json:"imageHashes"`
	Samples        []timing         `json:"samples"`
	PixelMatch     bool             `json:"pixelMatch"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	out := flag.String("out", "", "absolute output directory")
	mode := flag.String("mode", "partial", "full or partial")
	format := flag.String("format", "png", "png or jpeg (quality 85, clockwise 90 degrees)")
	warmup := flag.Int("warmup", 10, "warmup frames per style")
	iterations := flag.Int("iterations", 100, "measured frames per style")
	flag.Parse()
	if !filepath.IsAbs(*out) || (*mode != "full" && *mode != "partial") || (*format != "png" && *format != "jpeg") || *warmup < 0 || *iterations < 2 {
		return fmt.Errorf("specify absolute -out, -mode=full|partial, -format=png|jpeg, -warmup>=0 and -iterations>=2")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	started := time.Now()
	options := C.resvg_options_create()
	defer C.resvg_options_destroy(options)
	for _, name := range []string{"YuGothM.ttc", "YuGothB.ttc"} {
		path := C.CString(filepath.Join(os.Getenv("WINDIR"), "Fonts", name))
		code := C.resvg_options_load_font_file(options, path)
		C.free(unsafe.Pointer(path))
		if code != 0 {
			return fmt.Errorf("font %s: resvg code %d", name, code)
		}
	}
	family := C.CString("Yu Gothic")
	C.resvg_options_set_font_family(options, family)
	C.free(unsafe.Pointer(family))
	initializationMS := ms(time.Since(started))
	client := &http.Client{Timeout: 30 * time.Second}
	defer client.CloseIdleConnections()
	styles := make([]styleResult, 0, 2)
	for _, theme := range []string{"gauges", "bars"} {
		style := "Gauges"
		if theme == "bars" {
			style = "Bars"
		}
		setup := time.Now()
		entry := styleResult{Style: style, ReferenceRGBA: map[int]string{}, ReferenceImage: map[int]string{}, ImageHashes: map[int][]string{}, Samples: make([]timing, 0, *iterations)}
		var frame *image.RGBA
		// Only hashes of reference values are retained. They never supply output
		// frames or SVG trees to either measured rendering mode.
		for _, remaining := range []int{72, 71} {
			svg, _, err := get(client, "/full", theme, remaining)
			if err != nil {
				return err
			}
			reference, _, _, err := raster(options, svg, width, height)
			if err != nil {
				return err
			}
			entry.ReferenceRGBA[remaining] = digest(reference.Pix)
			encoded, _, _, err := encode(reference, *format)
			if err != nil {
				return err
			}
			entry.ReferenceImage[remaining] = digest(encoded)
			if remaining == 72 {
				frame = reference
			}
			if err := os.WriteFile(filepath.Join(*out, fmt.Sprintf("%s-reference-%d.%s", style, remaining, *format)), encoded, 0o644); err != nil {
				return err
			}
		}
		body, _, err := get(client, "/patch", theme, 72)
		if err != nil {
			return err
		}
		var expected patch
		if err := json.Unmarshal(body, &expected); err != nil {
			return err
		}
		entry.Region = image.Rect(expected.X, expected.Y, expected.X+expected.Width, expected.Y+expected.Height)
		entry.SetupMS = ms(time.Since(setup))
		measure := func(remaining int) (timing, []byte, error) {
			observed := time.Now()
			endpoint := "/full"
			if *mode == "partial" {
				endpoint = "/patch"
			}
			body, templateMS, err := get(client, endpoint, theme, remaining)
			if err != nil {
				return timing{}, nil, err
			}
			prepared := time.Now()
			var parseMS, renderMS, compositeMS float64
			if *mode == "partial" {
				var update patch
				if err := json.Unmarshal(body, &update); err != nil {
					return timing{}, nil, err
				}
				if update.LayoutKey != expected.LayoutKey || update.X != expected.X || update.Y != expected.Y || update.Width != expected.Width || update.Height != expected.Height {
					return timing{}, nil, fmt.Errorf("%s layout changed; retained background is invalid", style)
				}
				tile, parsed, rendered, err := raster(options, []byte(update.SVG), update.Width, update.Height)
				if err != nil {
					return timing{}, nil, err
				}
				parseMS, renderMS = parsed, rendered
				composite := time.Now()
				draw.Draw(frame, entry.Region, tile, image.Point{}, draw.Src)
				compositeMS = ms(time.Since(composite))
			} else {
				var err error
				frame, parseMS, renderMS, err = raster(options, body, width, height)
				if err != nil {
					return timing{}, nil, err
				}
			}
			encoded, rotateMS, encodeMS, err := encode(frame, *format)
			if err != nil {
				return timing{}, nil, err
			}
			finished := time.Now()
			return timing{TemplateMS: templateMS, ParseMS: parseMS, RenderMS: renderMS, CompositeMS: compositeMS, RotateMS: rotateMS, EncodeMS: encodeMS, TotalMS: templateMS + ms(finished.Sub(prepared)), ObservedMS: ms(finished.Sub(observed)), Bytes: len(encoded)}, encoded, nil
		}
		validate := func(remaining int, encoded []byte) error {
			if digest(frame.Pix) != entry.ReferenceRGBA[remaining] {
				return fmt.Errorf("%s %s at %d%% differs from the full SVG pixels", style, *mode, remaining)
			}
			if digest(encoded) != entry.ReferenceImage[remaining] {
				return fmt.Errorf("%s %s at %d%% differs from the full SVG encoded image", style, *mode, remaining)
			}
			config, decodedFormat, err := image.DecodeConfig(bytes.NewReader(encoded))
			wantWidth, wantHeight := width, height
			if *format == "jpeg" {
				wantWidth, wantHeight = height, width
			}
			if err != nil || decodedFormat != *format || config.Width != wantWidth || config.Height != wantHeight {
				return fmt.Errorf("invalid %s dimensions/format: %v %s %v", *format, config, decodedFormat, err)
			}
			return nil
		}
		first, encoded, err := measure(72)
		if err != nil {
			return err
		}
		if err := validate(72, encoded); err != nil {
			return err
		}
		entry.First = first
		for i := 0; i < *warmup; i++ {
			if _, _, err := measure(72 - i%2); err != nil {
				return err
			}
		}
		hashes := map[int]map[string]bool{72: {}, 71: {}}
		images := map[int][]byte{}
		for i := 0; i < *iterations; i++ {
			remaining := 72 - i%2
			sample, encoded, err := measure(remaining)
			if err != nil {
				return err
			}
			if err := validate(remaining, encoded); err != nil {
				return err
			}
			entry.Samples = append(entry.Samples, sample)
			hashes[remaining][digest(encoded)] = true
			images[remaining] = encoded
		}
		for hash := range hashes[72] {
			if hashes[71][hash] {
				return fmt.Errorf("%s returned the same image for different values", style)
			}
		}
		for remaining, imageData := range images {
			for hash := range hashes[remaining] {
				entry.ImageHashes[remaining] = append(entry.ImageHashes[remaining], hash)
			}
			if err := os.WriteFile(filepath.Join(*out, fmt.Sprintf("%s-%s-%d.%s", style, *mode, remaining, *format)), imageData, 0o644); err != nil {
				return err
			}
		}
		entry.PixelMatch = true
		styles = append(styles, entry)
		fmt.Printf("%s %s %s: %d frames match full SVG pixels and encoded hashes\n", style, *mode, *format, *iterations)
	}
	result := struct {
		Method           string        `json:"method"`
		Mode             string        `json:"mode"`
		Format           string        `json:"format"`
		ResvgVersion     string        `json:"resvgVersion"`
		Width            int           `json:"width"`
		Height           int           `json:"height"`
		EncodedWidth     int           `json:"encodedWidth"`
		EncodedHeight    int           `json:"encodedHeight"`
		Warmup           int           `json:"warmup"`
		Iterations       int           `json:"iterations"`
		InitializationMS float64       `json:"initializationMs"`
		Boundary         string        `json:"boundary"`
		Styles           []styleResult `json:"styles"`
	}{
		Method: "svg-resvg-region-update", Mode: *mode, Format: *format, ResvgVersion: "0.48.1", Width: width, Height: height, Warmup: *warmup, Iterations: *iterations, InitializationMS: initializationMS, Styles: styles,
		Boundary: "totalMs includes SVG data/geometry/template preparation and region extraction (Server-Timing), response JSON processing, fresh SVG parsing, native rendering and tree destruction, region replacement, optional clockwise 90-degree rotation, and Go PNG DefaultCompression or JPEG quality 85 encoding. HTTP is excluded from totalMs and included in observedMs. Partial mode retains the current full frame and replaces the complete Claude contract rectangle. Each update parses a newly generated SVG; previous 72/71 trees/images are never reused to render. Full mode uses the original full SVG template. Reference full renders, hashes, pixel/format/dimension checks, file writes and USB transfer are outside measurement. SetupMS includes verification reference generation and file writes, so it is not production startup time. Setup/reference preparation warms the renderer before first; first is the first measured frame after validation setup, not cold startup. Initialization is font/options setup only. Layout changes are rejected because this benchmark covers the stable 72/71 layout.",
	}
	result.EncodedWidth, result.EncodedHeight = width, height
	if *format == "jpeg" {
		result.EncodedWidth, result.EncodedHeight = height, width
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*out, fmt.Sprintf("resvg-%s-%s.json", *mode, *format)), append(data, '\n'), 0o644)
}

func get(client *http.Client, endpoint, theme string, remaining int) ([]byte, float64, error) {
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:9351%s?theme=%s&remaining=%d", endpoint, theme, remaining))
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, 0, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("SVG server returned %d: %s", response.StatusCode, data)
	}
	duration, err := strconv.ParseFloat(strings.TrimPrefix(response.Header.Get("Server-Timing"), "theme;dur="), 64)
	return data, duration, err
}

func raster(options *C.resvg_options, svg []byte, w, h int) (*image.RGBA, float64, float64, error) {
	started := time.Now()
	var tree *C.resvg_render_tree
	code := C.resvg_parse_tree_from_data((*C.char)(unsafe.Pointer(&svg[0])), C.uintptr_t(len(svg)), options, &tree)
	if code != 0 {
		return nil, 0, 0, fmt.Errorf("parse SVG: resvg code %d", code)
	}
	parsed := time.Now()
	size := C.resvg_get_image_size(tree)
	if int(size.width) != w || int(size.height) != h {
		C.resvg_tree_destroy(tree)
		return nil, 0, 0, fmt.Errorf("unexpected SVG dimensions: %v, expected %dx%d", size, w, h)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	C.resvg_render(tree, C.resvg_transform_identity(), C.uint32_t(w), C.uint32_t(h), (*C.char)(unsafe.Pointer(&img.Pix[0])))
	C.resvg_tree_destroy(tree)
	return img, ms(parsed.Sub(started)), ms(time.Since(parsed)), nil
}

func encode(img *image.RGBA, format string) ([]byte, float64, float64, error) {
	var rotateMS float64
	if format == "jpeg" {
		started := time.Now()
		rotated := image.NewRGBA(image.Rect(0, 0, height, width))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				source := y*img.Stride + x*4
				destination := x*rotated.Stride + (height-1-y)*4
				copy(rotated.Pix[destination:destination+4], img.Pix[source:source+4])
			}
		}
		img = rotated
		rotateMS = ms(time.Since(started))
	}
	started := time.Now()
	var buffer bytes.Buffer
	var err error
	if format == "png" {
		err = png.Encode(&buffer, img)
	} else {
		err = jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 85})
	}
	return buffer.Bytes(), rotateMS, ms(time.Since(started)), err
}

func digest(data []byte) string         { return fmt.Sprintf("%x", sha256.Sum256(data)) }
func ms(duration time.Duration) float64 { return float64(duration) / float64(time.Millisecond) }
