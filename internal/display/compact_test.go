package display

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"io"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

func compactWindow(label string, remaining *float64) usage.Window {
	return usage.Window{Kind: "session", Label: label, ShowMeter: true, RemainingPercent: remaining}
}

func percent(v float64) *float64 { return &v }

func TestCompactPagesKeepServiceOrderAndSplitContracts(t *testing.T) {
	low := usage.Provider{Provider: "alpha", AccountLabel: "low", Windows: []usage.Window{
		compactWindow("A 5-hour", percent(10)), compactWindow("B 5-hour", percent(20)),
		compactWindow("C 5-hour", percent(30)), compactWindow("D 5-hour", percent(40)),
		compactWindow("E 5-hour", percent(50)), {ShowMeter: false, RemainingPercent: percent(0)},
	}}
	stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{
		{Provider: "alpha", AccountLabel: "high", Windows: []usage.Window{compactWindow("5-hour", percent(80))}},
		{Provider: "beta", AccountLabel: "last", Windows: []usage.Window{compactWindow("5-hour", percent(1))}},
		low,
		{Provider: "alpha", AccountLabel: "tie", Windows: []usage.Window{compactWindow("5-hour", percent(10))}},
		{Provider: "alpha", AccountLabel: "unknown", Windows: []usage.Window{compactWindow("5-hour", nil)}},
	}}}
	for _, style := range []Style{Gauges, Bars} {
		t.Run(string(style), func(t *testing.T) {
			pages, message := compactPages(stats, Options{Style: style})
			var names, windows []string
			for _, page := range pages {
				names = append(names, page.provider.AccountLabel)
				if style == Gauges && len(page.panel.circles) > 3 || style == Bars && len(page.windows) > 3 {
					t.Fatalf("oversized page: %+v", page)
				}
				if page.provider.AccountLabel == "low" {
					ws := page.windows
					if style == Gauges {
						for _, circle := range page.panel.circles {
							ws = append(ws, circle.windows...)
						}
					}
					for _, w := range ws {
						windows = append(windows, w.Label)
					}
				}
			}
			want := []string{"low", "low", "tie", "high", "unknown", "last"}
			if message != "" || !reflect.DeepEqual(names, want) {
				t.Fatalf("pages %v (%q), want %v", names, message, want)
			}
			if !reflect.DeepEqual(windows, []string{"A 5-hour", "B 5-hour", "C 5-hour", "D 5-hour", "E 5-hour"}) {
				t.Fatalf("lost or repeated windows: %v", windows)
			}
		})
	}
}

func TestCompactSkipUsesVisibleExactFullFiveHourWindows(t *testing.T) {
	for _, tc := range []struct {
		name    string
		windows []usage.Window
		skipped bool
	}{
		{"exactly full", []usage.Window{compactWindow("5h", percent(100))}, true},
		{"rounded full", []usage.Window{compactWindow("5h", percent(99.9))}, false},
		{"unknown", []usage.Window{compactWindow("5h", nil)}, false},
		{"weekly only", []usage.Window{{Kind: "weekly", ShowMeter: true, RemainingPercent: percent(100)}}, false},
		{"reported five hour", []usage.Window{{Kind: "weekly", WindowMinutes: percent(300), ShowMeter: true, RemainingPercent: percent(100)}}, true},
		{"reported daily overrides session", []usage.Window{{Kind: "session", WindowMinutes: percent(1440), ShowMeter: true, RemainingPercent: percent(100)}}, false},
		{"hidden nonfull", []usage.Window{compactWindow("5h", percent(100)), {Kind: "session", ShowMeter: false, RemainingPercent: percent(20)}}, true},
		{"one unknown model", []usage.Window{compactWindow("A 5h", percent(100)), compactWindow("B 5h", nil)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{{Provider: "alpha", Windows: tc.windows}}}}
			pages, message := compactPages(stats, Options{Style: Bars, SkipFull5hServices: true})
			if (len(pages) == 0) != tc.skipped {
				t.Fatalf("pages %d, skipped want %v", len(pages), tc.skipped)
			}
			if tc.skipped && message != "All services skipped" {
				t.Fatalf("message %q", message)
			}
			if pages, _ := compactPages(stats, Options{Style: Bars}); len(pages) == 0 {
				t.Fatal("skip off excluded a service")
			}
		})
	}
	stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{
		{Provider: "alpha", AccountLabel: "first", Windows: []usage.Window{compactWindow("5h", percent(100))}},
		{Provider: "alpha", AccountLabel: "second", Windows: []usage.Window{compactWindow("5h", percent(99.9))}},
	}}}
	if pages, _ := compactPages(stats, Options{Style: Bars, SkipFull5hServices: true}); len(pages) != 2 {
		t.Fatalf("partly full service: %d pages", len(pages))
	}
	hidden := map[string]bool{windowKey(stats.Limits.Providers[1], stats.Limits.Providers[1].Windows[0]): true}
	if pages, message := compactPages(withoutHidden(stats, hidden), Options{Style: Bars, SkipFull5hServices: true}); len(pages) != 0 || message != "All services skipped" {
		t.Fatalf("hidden windows counted: %d, %q", len(pages), message)
	}
	if _, message := compactPages(nil, Options{}); message != "" {
		t.Fatalf("before data: %q", message)
	}
	if _, message := compactPages(&usage.Stats{}, Options{SkipFull5hServices: true}); message != "No services to display" {
		t.Fatalf("empty: %q", message)
	}
}

func TestCompactRotationKeepsDeadlineAndSurvivingSuccessor(t *testing.T) {
	now := time.Unix(1000, 0)
	options := Options{Compact: true, DeviceID: "compact", DeviceConnected: true, Orientation: "Landscape", Style: Bars, Interval: 10 * time.Second}
	pages := []compactPage{{key: "a"}, {key: "b"}, {key: "c"}}
	var r rotation
	r.update(pages, options, now)
	r.update(pages, options, now.Add(9*time.Second))
	if r.index != 0 || !r.deadline.Equal(now.Add(10*time.Second)) {
		t.Fatalf("data restarted deadline: %+v", r)
	}
	r.update(pages, options, now.Add(10*time.Second))
	if r.index != 1 {
		t.Fatalf("did not advance: %d", r.index)
	}
	r.update([]compactPage{pages[0], pages[2]}, options, now.Add(11*time.Second))
	if r.pages[r.index].key != "c" || !r.deadline.Equal(now.Add(20*time.Second)) {
		t.Fatalf("removed page lost successor/deadline: %+v", r)
	}
	options.DeviceConnected = false
	r.update(r.pages, options, now.Add(12*time.Second))
	options.DeviceConnected = true
	r.update(r.pages, options, now.Add(13*time.Second))
	if r.pages[r.index].key != "c" || !r.deadline.Equal(now.Add(20*time.Second)) {
		t.Fatal("reconnection restarted rotation")
	}
	r.update(r.pages, options, now.Add(20*time.Second))
	if r.index != 0 {
		t.Fatal("did not wrap")
	}
	r.update(nil, options, now.Add(21*time.Second))
	r.update(pages, options, now.Add(22*time.Second))
	if r.index != 0 || !r.deadline.Equal(now.Add(32*time.Second)) {
		t.Fatal("empty recovery did not restart")
	}
	for _, change := range []func(*Options){
		func(o *Options) { o.Orientation = "ReversePortrait" }, func(o *Options) { o.Style = Gauges },
		func(o *Options) { o.Interval = 30 * time.Second }, func(o *Options) { o.SkipFull5hServices = true },
		func(o *Options) { o.DeviceID = "other" },
	} {
		var cycle rotation
		cycle.update(pages, options, now)
		cycle.update(pages, options, now.Add(10*time.Second))
		next := options
		change(&next)
		cycle.update(pages, next, now.Add(11*time.Second))
		if cycle.index != 0 || !cycle.deadline.Equal(now.Add(11*time.Second).Add(next.Interval)) {
			t.Fatalf("settings did not restart: %+v", cycle)
		}
	}
}

func testCompactRenderer(t *testing.T) *Renderer {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("application requires Windows Yu Gothic")
	}
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCompactReversedPreviewsStayUpright(t *testing.T) {
	r := testCompactRenderer(t)
	stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{{Provider: "alpha", AccountLabel: "Account", Windows: []usage.Window{compactWindow("A 5h", percent(10)), compactWindow("B 5h", percent(80))}}}}}
	for _, style := range []Style{Gauges, Bars} {
		options := Options{Style: style}
		pages, _ := compactPages(stats, options)
		for _, pair := range [][2]string{{"Landscape", "ReverseLandscape"}, {"Portrait", "ReversePortrait"}} {
			options.Orientation = pair[0]
			base := r.renderCompact(stats, "Local", options, &pages[0], 1, 1, "", time.Unix(0, 0))
			options.Orientation = pair[1]
			reverse := r.renderCompact(stats, "Local", options, &pages[0], 1, 1, "", time.Unix(0, 0))
			if !bytes.Equal(base.Pix, reverse.Pix) {
				t.Fatalf("%s/%s changed content", style, pair[1])
			}
			w, h := 480, 320
			if pair[0] == "Portrait" {
				w, h = 320, 480
			}
			if reverse.Bounds().Dx() != w || reverse.Bounds().Dy() != h {
				t.Fatal("wrong compact dimensions")
			}
		}
	}
}

func TestDisplayRunReconnectsWithCurrentPageAndMatchingPreview(t *testing.T) {
	r := testCompactRenderer(t)
	state := usage.NewState()
	stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{
		{Provider: "alpha", Windows: []usage.Window{compactWindow("5h", percent(10))}},
		{Provider: "beta", Windows: []usage.Window{compactWindow("5h", percent(80))}},
	}}}
	state.Set(stats)
	options := Options{DeviceID: "compact", DeviceConnected: true, Compact: true, Orientation: "ReversePortrait", Style: Bars, Interval: 600 * time.Millisecond}
	var mu sync.Mutex
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Service{State: state, Hidden: func() ([]string, error) { return nil, nil }, Options: func() (Options, error) { mu.Lock(); defer mu.Unlock(); return options, nil }}
	type result struct {
		frame   Frame
		preview string
	}
	frames := make(chan result, 32)
	canvasResult := canvasFrame(t)
	var wideRequest *FrameRequest
	canvas := NewCanvasRenderer(s, func(string, any) {
		wideRequest = s.RenderRequest()
		if err := s.CompleteFrame(wideRequest.ID, base64.StdEncoding.EncodeToString(canvasResult.PNG), base64.StdEncoding.EncodeToString(canvasResult.JPEG), ""); err != nil {
			t.Error(err)
		}
	})
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		var latest Frame
		Run(ctx, s, canvas, r, state, time.Minute, func() Style { return Bars }, func(f Frame) { latest = f }, func(string, any) { frames <- result{latest, s.Preview()} }, logger)
	}()
	defer func() { stop(); <-done }()
	deadline := time.Now().Add(3 * time.Second)
	get := func() result {
		t.Helper()
		select {
		case f := <-frames:
			return f
		case <-time.After(time.Until(deadline)):
			t.Fatal("no current frame")
			return result{}
		}
	}
	first, second := get(), get()
	for bytes.Equal(first.frame.Image.Pix, second.frame.Image.Pix) {
		second = get()
	}
	mu.Lock()
	options.DeviceConnected = false
	mu.Unlock()
	state.Touch()
	disconnected := get()
	for disconnected.frame.Target.DeviceConnected {
		disconnected = get()
	}
	mu.Lock()
	options.DeviceID, options.Compact = "", false
	mu.Unlock()
	state.Touch()
	absent := get()
	for absent.frame.Target.DeviceID != "" {
		absent = get()
	}
	if !absent.frame.Target.Compact || !bytes.Equal(second.frame.Image.Pix, absent.frame.Image.Pix) {
		t.Fatal("Automatic absence lost the current compact page")
	}
	mu.Lock()
	options.DeviceID, options.Compact = "compact", true
	options.DeviceConnected = true
	mu.Unlock()
	state.Touch()
	reconnected := get()
	for !reconnected.frame.Target.DeviceConnected {
		reconnected = get()
	}
	if disconnected.frame.Target.DeviceConnected || !reconnected.frame.Target.DeviceConnected || !bytes.Equal(second.frame.Image.Pix, reconnected.frame.Image.Pix) {
		t.Fatal("reconnection replayed the first page")
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, reconnected.frame.Image); err != nil {
		t.Fatal(err)
	}
	preview, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(reconnected.preview, "data:image/png;base64,"))
	if err != nil || !bytes.Equal(preview, encoded.Bytes()) {
		t.Fatal("preview is not the submitted image")
	}
	updated := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{
		stats.Limits.Providers[0], {Provider: "beta", Windows: []usage.Window{compactWindow("5h", percent(75))}},
	}}}
	state.Set(updated)
	latest := get()
	for bytes.Equal(latest.frame.Image.Pix, reconnected.frame.Image.Pix) {
		latest = get()
	}
	pages, _ := compactPages(updated, options)
	expected := testCompactRenderer(t).renderCompact(updated, "Local", options, &pages[1], 2, 2, "", time.Unix(0, 0))
	if !bytes.Equal(latest.frame.Image.Pix, expected.Pix) {
		t.Fatal("latest data did not update the current page")
	}
	mu.Lock()
	options.Compact = false
	options.DeviceID = "wide"
	options.SkipFull5hServices = true
	options.ServiceContent = &serviceSelection{content: map[string]ServiceContent{"alpha": {}, "beta": {}}}
	mu.Unlock()
	state.Touch()
	wide := get()
	for wide.frame.Target.Compact {
		wide = get()
	}
	if wide.frame.Target.Compact || !bytes.Equal(wide.frame.PNG, canvasResult.PNG) || !bytes.Equal(wide.frame.JPEG, canvasResult.JPEG) {
		t.Fatal("compact orientation affected the wide Canvas result")
	}
	if wideRequest.Theme != "bars" || !reflect.DeepEqual(wideRequest.Data, themeData(updated, time.Unix(0, 0), "Local")) {
		t.Fatal("compact content/skip settings affected the wide display data")
	}
}

func TestOutputRetainsOnlyLatestFrameAndItsDestination(t *testing.T) {
	o := NewOutput(nil, nil, nil)
	for _, id := range []string{"wide", "compact-old", "compact-new"} {
		o.Submit(Frame{Target: Options{DeviceID: id}})
	}
	if len(o.pending) != 1 {
		t.Fatalf("pending images: %d", len(o.pending))
	}
	if got := <-o.pending; got.Target.DeviceID != "compact-new" {
		t.Fatalf("stale destination: %+v", got.Target)
	}
}
