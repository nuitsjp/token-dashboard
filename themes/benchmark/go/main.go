// Command go benchmarks the existing renderer against the theme preview fixture.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"token-monitor-turzx/internal/display"
	"token-monitor-turzx/internal/usage"
)

const (
	fixedNow = "2026-10-09T12:00:00+09:00"
)

type fixture struct {
	Tokens []struct {
		Label      string `json:"label"`
		TokensText string `json:"tokensText"`
		CostText   string `json:"costText"`
	} `json:"tokens"`
	Contracts []struct {
		Provider string `json:"provider"`
		Plan     string `json:"plan"`
		Circles  []struct {
			Windows []struct {
				Label            string   `json:"label"`
				DurationLabel    string   `json:"durationLabel"`
				RemainingPercent *float64 `json:"remainingPercent"`
				ResetText        string   `json:"resetText"`
			} `json:"windows"`
		} `json:"circles"`
	} `json:"contracts"`
}

type timing struct {
	RenderMS   float64 `json:"renderMs"`
	RotateMS   float64 `json:"rotateMs"`
	EncodeMS   float64 `json:"encodeMs"`
	TotalMS    float64 `json:"totalMs"`
	ImageBytes int     `json:"imageBytes"`
}

type styleResult struct {
	Style            display.Style    `json:"style"`
	InitializationMS float64          `json:"initializationMs"`
	First            timing           `json:"first"`
	Samples          []timing         `json:"samples"`
	ImageHashes      map[int][]string `json:"imageHashes"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	out := flag.String("out", "", "output directory for generated images")
	format := flag.String("format", "png", "png or jpeg (clockwise rotated, quality 85)")
	iterations := flag.Int("iterations", 100, "measured frames per style")
	warmup := flag.Int("warmup", 10, "warmup frames per style")
	flag.Parse()
	if (*format != "png" && *format != "jpeg") || *iterations < 2 || *warmup < 0 {
		return fmt.Errorf("expected png/jpeg, iterations >= 2 and warmup >= 0")
	}
	extension := "png"
	outputWidth, outputHeight := display.Width, display.Height
	if *format == "jpeg" {
		extension = "jpg"
		outputWidth, outputHeight = display.Height, display.Width
	}
	now, err := time.Parse(time.RFC3339, fixedNow)
	if err != nil {
		return err
	}
	data, err := os.ReadFile("themes/preview/sample-data.json")
	if err != nil {
		return err
	}
	var sample fixture
	if err := json.Unmarshal(data, &sample); err != nil {
		return err
	}
	var stats usage.Stats
	periods := map[string]*usage.Period{
		"Today": &stats.Periods.Today,
		"Month": &stats.Periods.Month,
		"All":   &stats.Periods.AllTime,
	}
	for _, token := range sample.Tokens {
		period, ok := periods[token.Label]
		if !ok {
			return fmt.Errorf("unknown token period %q", token.Label)
		}
		period.TotalTokens, err = strconv.ParseInt(strings.ReplaceAll(token.TokensText, ",", ""), 10, 64)
		if err != nil {
			return fmt.Errorf("%s tokens: %w", token.Label, err)
		}
		period.CostUSD, err = strconv.ParseFloat(strings.ReplaceAll(strings.TrimPrefix(token.CostText, "$"), ",", ""), 64)
		if err != nil {
			return fmt.Errorf("%s cost: %w", token.Label, err)
		}
	}
	kinds := map[string]string{"5h": "session", "7d": "weekly", "1d": "daily", "1mo": "billing"}
	for _, contract := range sample.Contracts {
		provider := usage.Provider{Provider: contract.Provider, PlanLabel: contract.Plan}
		for _, circle := range contract.Circles {
			for _, window := range circle.Windows {
				kind, ok := kinds[window.DurationLabel]
				if !ok {
					return fmt.Errorf("unknown window duration %q", window.DurationLabel)
				}
				var resetsAt *time.Time
				if window.ResetText != "—" {
					text := strings.ReplaceAll(window.ResetText, " ", "")
					var duration time.Duration
					if daysText, tail, hasDays := strings.Cut(text, "d"); hasDays {
						days, err := strconv.Atoi(daysText)
						if err != nil {
							return fmt.Errorf("%s reset days: %w", window.Label, err)
						}
						duration = time.Duration(days) * 24 * time.Hour
						text = tail
					}
					if text != "" {
						rest, err := time.ParseDuration(text)
						if err != nil {
							return fmt.Errorf("%s reset time: %w", window.Label, err)
						}
						duration += rest
					}
					reset := now.Add(duration)
					resetsAt = &reset
				}
				provider.Windows = append(provider.Windows, usage.Window{
					Kind:             kind,
					Label:            window.Label,
					ShowMeter:        true,
					RemainingPercent: window.RemainingPercent,
					ResetsAt:         resetsAt,
				})
			}
		}
		stats.Limits.Providers = append(stats.Limits.Providers, provider)
	}

	result := struct {
		Method       string        `json:"method"`
		Format       string        `json:"format"`
		Now          string        `json:"now"`
		Width        int           `json:"width"`
		Height       int           `json:"height"`
		OutputWidth  int           `json:"outputWidth"`
		OutputHeight int           `json:"outputHeight"`
		Warmup       int           `json:"warmup"`
		Iterations   int           `json:"iterations"`
		Boundary     string        `json:"boundary"`
		Styles       []styleResult `json:"styles"`
	}{
		Method: "go", Format: *format, Now: fixedNow, Width: display.Width, Height: display.Height,
		OutputWidth: outputWidth, OutputHeight: outputHeight, Warmup: *warmup, Iterations: *iterations,
		Boundary: "initializationMs measures display.NewRenderer. Each sample measures Renderer.Render, clockwise rotation for JPEG, and encoding into a fresh in-memory buffer. PNG uses Go image/png DefaultCompression. JPEG uses Go image/jpeg Quality 85 at 462x1920. Fixture loading/conversion, remaining-value updates, validation, hashing, file writes, JSON output and USB transfer are excluded. Samples alternate claude-session remaining percent between 72 and 71.",
	}
	for _, style := range []display.Style{display.Gauges, display.Bars} {
		value := 72.0
		stats.Limits.Providers[0].Windows[0].RemainingPercent = &value
		started := time.Now()
		renderer, err := display.NewRenderer()
		initialization := time.Since(started)
		if err != nil {
			return err
		}
		measure := func() (timing, []byte, error) {
			start := time.Now()
			img := renderer.Render(&stats, now, "Hub", style)
			rendered := time.Now()
			if *format == "jpeg" {
				img = rotateClockwise(img)
			}
			rotated := time.Now()
			var buffer bytes.Buffer
			var err error
			if *format == "png" {
				err = png.Encode(&buffer, img)
			} else {
				err = jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 85})
			}
			encoded := time.Now()
			return timing{
				RenderMS:   float64(rendered.Sub(start)) / float64(time.Millisecond),
				RotateMS:   float64(rotated.Sub(rendered)) / float64(time.Millisecond),
				EncodeMS:   float64(encoded.Sub(rotated)) / float64(time.Millisecond),
				TotalMS:    float64(encoded.Sub(start)) / float64(time.Millisecond),
				ImageBytes: buffer.Len(),
			}, buffer.Bytes(), err
		}
		first, firstPNG, err := measure()
		if err != nil {
			return err
		}
		if *out != "" {
			if err := os.MkdirAll(*out, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(*out, string(style)+"-go-first."+extension), firstPNG, 0o644); err != nil {
				return err
			}
		}
		for i := 0; i < *warmup; i++ {
			value = 72 - float64(i%2)
			if _, _, err := measure(); err != nil {
				return err
			}
		}
		entry := styleResult{
			Style: style, InitializationMS: float64(initialization) / float64(time.Millisecond),
			First: first, Samples: make([]timing, 0, *iterations), ImageHashes: map[int][]string{},
		}
		images := map[int][]byte{}
		hashes := map[int]map[string]bool{71: {}, 72: {}}
		for i := 0; i < *iterations; i++ {
			value = 72 - float64(i%2)
			measurement, encodedImage, err := measure()
			if err != nil {
				return err
			}
			entry.Samples = append(entry.Samples, measurement)
			config, _, err := image.DecodeConfig(bytes.NewReader(encodedImage))
			if err != nil || config.Width != outputWidth || config.Height != outputHeight {
				return fmt.Errorf("invalid output dimensions: %v", err)
			}
			remaining := int(value)
			images[remaining] = encodedImage
			hashes[remaining][fmt.Sprintf("%x", sha256.Sum256(encodedImage))] = true
		}
		for hash := range hashes[72] {
			if hashes[71][hash] {
				return fmt.Errorf("%s: different values produced identical images", style)
			}
		}
		for remaining, encodedImage := range images {
			if *out != "" {
				if err := os.WriteFile(filepath.Join(*out, fmt.Sprintf("%s-go-%d.%s", style, remaining, extension)), encodedImage, 0o644); err != nil {
					return err
				}
			}
			for hash := range hashes[remaining] {
				entry.ImageHashes[remaining] = append(entry.ImageHashes[remaining], hash)
			}
		}
		result.Styles = append(result.Styles, entry)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// Same pixel mapping as internal/display/output.go; kept local to this benchmark.
func rotateClockwise(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			s := src.PixOffset(b.Min.X+x, b.Min.Y+y)
			d := dst.PixOffset(b.Dy()-1-y, x)
			copy(dst.Pix[d:d+4], src.Pix[s:s+4])
		}
	}
	return dst
}
