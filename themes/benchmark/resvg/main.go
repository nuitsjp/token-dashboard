//go:build windows && resvgbench

// Command resvg benchmarks native SVG rendering using the same PNG encoder as the Go baseline.
// Build with -tags resvgbench and CGO_CFLAGS/CGO_LDFLAGS pointing at resvg 0.48.1.
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

const (
	width      = 1920
	height     = 462
	warmup     = 10
	iterations = 100
)

type timing struct {
	TemplateMS float64 `json:"templateMs"`
	ParseMS    float64 `json:"parseMs"`
	RenderMS   float64 `json:"renderMs"`
	EncodeMS   float64 `json:"encodeMs"`
	TotalMS    float64 `json:"totalMs"`
	ObservedMS float64 `json:"observedMs"`
	PNGBytes   int     `json:"pngBytes"`
}

type styleResult struct {
	Style     string              `json:"style"`
	First     timing              `json:"first"`
	PNGHashes map[string][]string `json:"pngHashes"`
	Samples   []timing            `json:"samples"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	out := flag.String("out", "", "absolute output directory for PNGs, SVGs and resvg.json")
	flag.Parse()
	if *out == "" || !filepath.IsAbs(*out) {
		return fmt.Errorf("-out must specify an absolute output directory")
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
			return fmt.Errorf("load font %s: resvg code %d", name, code)
		}
	}
	family := C.CString("Yu Gothic")
	C.resvg_options_set_font_family(options, family)
	C.free(unsafe.Pointer(family))
	initializationMS := milliseconds(time.Since(started))
	client := &http.Client{Timeout: 30 * time.Second}
	defer client.CloseIdleConnections()
	result := struct {
		Method           string        `json:"method"`
		ResvgVersion     string        `json:"resvgVersion"`
		PNGEncoder       string        `json:"pngEncoder"`
		Width            int           `json:"width"`
		Height           int           `json:"height"`
		Warmup           int           `json:"warmup"`
		Iterations       int           `json:"iterations"`
		InitializationMS float64       `json:"initializationMs"`
		Boundary         string        `json:"boundary"`
		Styles           []styleResult `json:"styles"`
	}{
		Method: "handlebars-resvg-go-png", ResvgVersion: "0.48.1", PNGEncoder: "Go image/png DefaultCompression",
		Width: width, Height: height, Warmup: warmup, Iterations: iterations, InitializationMS: initializationMS,
		Boundary: "totalMs = server-side SVG display-data/geometry preparation and Handlebars expansion + fresh SVG parsing/text shaping + bitmap allocation/native resvg rendering/tree destruction + Go png.Encode into a fresh memory buffer. The native library and Yu Gothic medium/bold font database stay loaded. Every sample creates a fresh SVG tree and bitmap; no rendered frame is cached. HTTP transport is excluded from totalMs and included in observedMs. Samples alternate claude-session remaining between 72 and 71. PNG uses the same Go encoder/default compression as the Go baseline. File writes, validation, hashes and USB transfer are excluded. Initialization measures options/font setup, excluding process startup and OS DLL loading.",
	}
	for _, id := range []string{"gauges", "bars"} {
		style := "Gauges"
		if id == "bars" {
			style = "Bars"
		}
		measure := func(remaining int) (timing, []byte, []byte, error) {
			return measureFrame(client, options, id, remaining)
		}
		first, firstPNG, firstSVG, err := measure(72)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*out, style+"-resvg-first.png"), firstPNG, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*out, style+"-resvg-first.svg"), firstSVG, 0o644); err != nil {
			return err
		}
		for i := 0; i < warmup; i++ {
			if _, _, _, err := measure(72 - i%2); err != nil {
				return err
			}
		}
		entry := styleResult{Style: style, First: first, PNGHashes: map[string][]string{}, Samples: make([]timing, 0, iterations)}
		images := map[int][]byte{}
		hashes := map[int]map[string]bool{71: {}, 72: {}}
		for i := 0; i < iterations; i++ {
			remaining := 72 - i%2
			measurement, pngData, _, err := measure(remaining)
			if err != nil {
				return err
			}
			entry.Samples = append(entry.Samples, measurement)
			config, err := png.DecodeConfig(bytes.NewReader(pngData))
			if err != nil || config.Width != width || config.Height != height {
				return fmt.Errorf("%s returned invalid PNG dimensions: %v", style, err)
			}
			images[remaining] = pngData
			hash := fmt.Sprintf("%x", sha256.Sum256(pngData))
			hashes[remaining][hash] = true
		}
		for hash := range hashes[72] {
			if hashes[71][hash] {
				return fmt.Errorf("%s returned the same PNG for different remaining values", style)
			}
		}
		for remaining, pngData := range images {
			if err := os.WriteFile(filepath.Join(*out, fmt.Sprintf("%s-resvg-%d.png", style, remaining)), pngData, 0o644); err != nil {
				return err
			}
			key := strconv.Itoa(remaining)
			for hash := range hashes[remaining] {
				entry.PNGHashes[key] = append(entry.PNGHashes[key], hash)
			}
		}
		result.Styles = append(result.Styles, entry)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, "resvg.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("resvg %s: 100 dynamic PNG captures validated for each style\n", result.ResvgVersion)
	return nil
}

func measureFrame(client *http.Client, options *C.resvg_options, style string, remaining int) (timing, []byte, []byte, error) {
	observed := time.Now()
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:9350/?theme=%s&remaining=%d", style, remaining))
	if err != nil {
		return timing{}, nil, nil, err
	}
	svg, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		return timing{}, nil, nil, err
	}
	if response.StatusCode != http.StatusOK {
		return timing{}, nil, nil, fmt.Errorf("SVG request: %d %s", response.StatusCode, svg)
	}
	templateMS, err := strconv.ParseFloat(strings.TrimPrefix(response.Header.Get("Server-Timing"), "theme;dur="), 64)
	if err != nil {
		return timing{}, nil, nil, err
	}
	started := time.Now()
	var tree *C.resvg_render_tree
	code := C.resvg_parse_tree_from_data((*C.char)(unsafe.Pointer(&svg[0])), C.uintptr_t(len(svg)), options, &tree)
	if code != 0 {
		return timing{}, nil, nil, fmt.Errorf("parse %s SVG: resvg code %d", style, code)
	}
	parsed := time.Now()
	size := C.resvg_get_image_size(tree)
	if size.width != width || size.height != height {
		C.resvg_tree_destroy(tree)
		return timing{}, nil, nil, fmt.Errorf("SVG image size: %v", size)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	C.resvg_render(tree, C.resvg_transform_identity(), width, height, (*C.char)(unsafe.Pointer(&img.Pix[0])))
	C.resvg_tree_destroy(tree)
	rendered := time.Now()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return timing{}, nil, nil, err
	}
	encoded := time.Now()
	return timing{
		TemplateMS: templateMS, ParseMS: milliseconds(parsed.Sub(started)), RenderMS: milliseconds(rendered.Sub(parsed)),
		EncodeMS: milliseconds(encoded.Sub(rendered)), TotalMS: templateMS + milliseconds(encoded.Sub(started)),
		ObservedMS: milliseconds(encoded.Sub(observed)), PNGBytes: buffer.Len(),
	}, buffer.Bytes(), svg, nil
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
