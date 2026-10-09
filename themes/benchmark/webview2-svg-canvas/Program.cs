using System.Diagnostics;
using System.Globalization;
using System.Security.Cryptography;
using System.Text.Json;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.WinForms;

internal static class Program
{
    [STAThread]
    private static void Main(string[] args)
    {
        var options = Options.Parse(args);
        Application.SetHighDpiMode(HighDpiMode.DpiUnaware);
        Application.EnableVisualStyles();
        using var form = new BenchmarkForm(options);
        Application.Run(form);
    }
}

internal sealed record Options(string OutputDirectory, string Format, int Iterations, int Warmup)
{
    public static Options Parse(string[] args)
    {
        string? output = null;
        var format = "png";
        var iterations = 100;
        var warmup = 10;
        for (var i = 0; i < args.Length; i += 2)
        {
            if (i + 1 >= args.Length)
                throw new ArgumentException("Usage: --out <directory> [--format png|jpeg] [--iterations 100] [--warmup 10]");
            switch (args[i])
            {
                case "--out": output = Path.GetFullPath(args[i + 1]); break;
                case "--format": format = args[i + 1]; break;
                case "--iterations": iterations = int.Parse(args[i + 1], CultureInfo.InvariantCulture); break;
                case "--warmup": warmup = int.Parse(args[i + 1], CultureInfo.InvariantCulture); break;
                default: throw new ArgumentException($"Unknown argument: {args[i]}");
            }
        }
        if (output is null || (format != "png" && format != "jpeg") || iterations < 2 || warmup < 0)
            throw new ArgumentException("Specify --out, png or jpeg, at least two iterations and a nonnegative warmup.");
        return new Options(output, format, iterations, warmup);
    }
}

internal sealed class BenchmarkForm : Form
{
    private readonly Options options;
    private readonly WebView2 view = new() { Dock = DockStyle.Fill };
    private readonly HttpClient http = new();
    private readonly JsonSerializerOptions jsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        WriteIndented = true,
    };

    public BenchmarkForm(Options options)
    {
        this.options = options;
        AutoScaleMode = AutoScaleMode.None;
        FormBorderStyle = FormBorderStyle.None;
        ClientSize = new Size(1920, 462);
        StartPosition = FormStartPosition.Manual;
        Location = new Point(-10000, -10000);
        ShowInTaskbar = false;
        Controls.Add(view);
    }

    protected override bool ShowWithoutActivation => true;

    protected override async void OnShown(EventArgs e)
    {
        base.OnShown(e);
        try { await RunAsync(); }
        catch (Exception error)
        {
            Console.Error.WriteLine(error);
            Environment.ExitCode = 1;
        }
        finally
        {
            http.Dispose();
            Close();
        }
    }

    private async Task RunAsync()
    {
        Directory.CreateDirectory(options.OutputDirectory);
        var startup = Stopwatch.StartNew();
        var environment = await CoreWebView2Environment.CreateAsync(
            userDataFolder: Path.Combine(options.OutputDirectory, "WebView2SvgCanvasProfile-" + options.Format));
        await view.EnsureCoreWebView2Async(environment);
        view.ZoomFactor = 1;
        var initializationMs = startup.Elapsed.TotalMilliseconds;
        await NavigateAsync();
        var geometry = await ScriptAsync("""
            if (devicePixelRatio !== 1) throw new Error('Wrong device pixel ratio');
            window.canvas = document.createElement('canvas');
            canvas.width = 1920; canvas.height = 462;
            document.body.append(canvas);
            window.context = canvas.getContext('2d', { alpha: false });
            window.rotated = document.createElement('canvas');
            rotated.width = 462; rotated.height = 1920;
            window.rotatedContext = rotated.getContext('2d', { alpha: false });
            await document.fonts.ready;
            return { width: canvas.width, height: canvas.height, devicePixelRatio, userAgent: navigator.userAgent };
            """);
        var styles = new List<StyleResult>();
        foreach (var id in new[] { "gauges", "bars" })
        {
            var style = id == "gauges" ? "Gauges" : "Bars";
            var (firstTiming, firstImage) = await MeasureAsync(id, 72);
            ValidateImage(firstImage);
            await SaveImageAsync(style, "first", firstImage);
            for (var i = 0; i < options.Warmup; i++)
                await MeasureAsync(id, 72 - i % 2);
            var samples = new List<Timing>();
            var images = new Dictionary<int, byte[]>();
            var hashes = new Dictionary<int, HashSet<string>>();
            for (var i = 0; i < options.Iterations; i++)
            {
                var remaining = 72 - i % 2;
                var (timing, image) = await MeasureAsync(id, remaining);
                samples.Add(timing);
                ValidateImage(image);
                images[remaining] = image;
                if (!hashes.TryGetValue(remaining, out var values)) hashes[remaining] = values = [];
                values.Add(Convert.ToHexString(SHA256.HashData(image)));
            }
            if (hashes[72].Overlaps(hashes[71]))
                throw new InvalidOperationException($"{style}: different values produced identical output images");
            foreach (var (remaining, image) in images)
                await SaveImageAsync(style, remaining.ToString(CultureInfo.InvariantCulture), image);
            styles.Add(new StyleResult(style, firstTiming, hashes.ToDictionary(
                pair => pair.Key.ToString(CultureInfo.InvariantCulture), pair => pair.Value.ToArray()), samples));
            Console.WriteLine($"{style}: {samples.Count} SVG/Canvas {options.Format} frames validated");
        }
        var result = new
        {
            Method = "handlebars-svg-webview2-canvas",
            RuntimeVersion = environment.BrowserVersionString,
            SdkVersion = "1.0.4258.31",
            Format = options.Format,
            Width = options.Format == "png" ? 1920 : 462,
            Height = options.Format == "png" ? 462 : 1920,
            JpegQuality = options.Format == "jpeg" ? 0.85 : (double?)null,
            Rotation = options.Format == "jpeg" ? "90 degrees clockwise" : "none",
            options.Warmup,
            options.Iterations,
            InitializationMs = initializationMs,
            Geometry = geometry,
            StopwatchFrequency = Stopwatch.Frequency,
            WindowMode = "Visible borderless parent HWND at (-10000, -10000), without activation. No browser flags. DPI unaware host, zoom 1, verified devicePixelRatio 1.",
            Boundary = "totalMs = server-side display-data/Handlebars generation + native JSON serialization/ExecuteScriptAsync + fresh SVG Blob/Image.decode + Canvas drawImage/optional clockwise rotation + Canvas.toBlob + FileReader base64 encoding + WebMessageReceived synchronization/JSON parse + native base64 decoding. HTTP transport is excluded from totalMs and included in observedMs. A new SVG image is parsed every frame; no frame, bitmap or SVG-tree cache. Canvas surfaces are reused, system fonts and shared icon byte sources are warmed. SVG parsing/embedded-image decoding/font work is represented by decodeMs; deferred rasterization may also occur in drawMs/encodeMs. ipcAndDispatchMs is the native pipeline time less JavaScript phase timings and native decoding; it includes dispatch, native input JSON serialization, postMessage/JSON parsing and scheduling. Template preparation, SVG decoding and encoding are not overlapped between frames. Validation/hash/file writes/USB are excluded. JPEG uses browser quality 0.85 and 90-degree clockwise rotation to 462x1920; PNG is unrotated 1920x462. Native/browser initialization is reported separately.",
            Styles = styles,
        };
        await File.WriteAllTextAsync(Path.Combine(options.OutputDirectory, $"webview2-svg-canvas-{options.Format}.json"),
            JsonSerializer.Serialize(result, jsonOptions));
    }

    private Task SaveImageAsync(string style, string value, byte[] image) => File.WriteAllBytesAsync(
        Path.Combine(options.OutputDirectory, $"{style}-webview2-svg-canvas-{value}.{options.Format}"), image);

    private async Task NavigateAsync()
    {
        var completion = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        void Completed(object? sender, CoreWebView2NavigationCompletedEventArgs e)
        {
            if (e.IsSuccess) completion.TrySetResult();
            else completion.TrySetException(new InvalidOperationException($"Navigation failed: {e.WebErrorStatus}"));
        }
        view.CoreWebView2.NavigationCompleted += Completed;
        try
        {
            view.CoreWebView2.NavigateToString("<!doctype html><html><head><meta charset='utf-8'></head><body style='margin:0'></body></html>");
            await completion.Task.WaitAsync(TimeSpan.FromSeconds(30));
        }
        finally { view.CoreWebView2.NavigationCompleted -= Completed; }
    }

    // ExecuteScriptAsync does not await Promises; the WebMessage is the completion barrier.
    private async Task<JsonElement> ScriptAsync(string script)
    {
        var id = Guid.NewGuid().ToString("N");
        var completion = new TaskCompletionSource<JsonElement>(TaskCreationOptions.RunContinuationsAsynchronously);
        void Received(object? sender, CoreWebView2WebMessageReceivedEventArgs e)
        {
            using var message = JsonDocument.Parse(e.WebMessageAsJson);
            var root = message.RootElement;
            if (root.GetProperty("id").GetString() != id) return;
            if (root.GetProperty("ok").GetBoolean()) completion.TrySetResult(root.GetProperty("result").Clone());
            else completion.TrySetException(new InvalidOperationException(root.GetProperty("error").GetString()));
        }
        view.CoreWebView2.WebMessageReceived += Received;
        try
        {
            await view.CoreWebView2.ExecuteScriptAsync($$"""
                (async () => {
                    try {
                        const result = await (async () => { {{script}} })();
                        chrome.webview.postMessage({ id: '{{id}}', ok: true, result });
                    } catch (error) {
                        chrome.webview.postMessage({ id: '{{id}}', ok: false, error: String(error) });
                    }
                })();
                """);
            return await completion.Task.WaitAsync(TimeSpan.FromSeconds(30));
        }
        finally { view.CoreWebView2.WebMessageReceived -= Received; }
    }

    private async Task<(Timing, byte[])> MeasureAsync(string id, int remaining)
    {
        var clock = Stopwatch.StartNew();
        using var response = await http.GetAsync($"http://127.0.0.1:9350/?theme={id}&remaining={remaining}");
        response.EnsureSuccessStatusCode();
        var svg = await response.Content.ReadAsStringAsync();
        var templateMs = double.Parse(response.Headers.GetValues("Server-Timing").Single().Split("dur=")[1], CultureInfo.InvariantCulture);
        var pipelineStarted = clock.Elapsed.TotalMilliseconds;
        var markup = JsonSerializer.Serialize(svg);
        var rotated = options.Format == "jpeg" ? "true" : "false";
        var encoded = await ScriptAsync($$"""
            const started = performance.now();
            const svg = new Blob([{{markup}}], { type: 'image/svg+xml;charset=utf-8' });
            const url = URL.createObjectURL(svg);
            const image = new Image();
            try {
                image.src = url;
                await image.decode();
                if (image.naturalWidth !== 1920 || image.naturalHeight !== 462)
                    throw new Error('Unexpected SVG image size');
                const decoded = performance.now();
                context.fillStyle = '#000'; context.fillRect(0, 0, 1920, 462);
                context.drawImage(image, 0, 0);
                const drawn = performance.now();
                let target = canvas;
                if ({{rotated}}) {
                    rotatedContext.setTransform(1, 0, 0, 1, 0, 0);
                    rotatedContext.fillStyle = '#000'; rotatedContext.fillRect(0, 0, 462, 1920);
                    rotatedContext.translate(462, 0);
                    rotatedContext.rotate(Math.PI / 2);
                    rotatedContext.drawImage(canvas, 0, 0);
                    target = rotated;
                }
                const rotatedAt = performance.now();
                const blob = await new Promise((resolve, reject) => target.toBlob(
                    blob => blob ? resolve(blob) : reject(new Error('Canvas.toBlob returned null')),
                    'image/{{options.Format}}', 0.85));
                const compressed = performance.now();
                const dataUrl = await new Promise((resolve, reject) => {
                    const reader = new FileReader();
                    reader.onload = () => resolve(reader.result);
                    reader.onerror = () => reject(reader.error);
                    reader.readAsDataURL(blob);
                });
                const transferred = performance.now();
                return { decodeMs: decoded - started, drawMs: drawn - decoded,
                    rotateMs: rotatedAt - drawn, encodeMs: compressed - rotatedAt,
                    base64Ms: transferred - compressed, jsTotalMs: transferred - started,
                    dataUrl, mimeType: blob.type, remaining: {{remaining}} };
            } finally { URL.revokeObjectURL(url); }
            """);
        if (encoded.GetProperty("mimeType").GetString() != "image/" + options.Format)
            throw new InvalidOperationException("Canvas returned an unexpected image format");
        var nativeDecodeStarted = clock.Elapsed.TotalMilliseconds;
        var dataUrl = encoded.GetProperty("dataUrl").GetString()!;
        var bytes = Convert.FromBase64String(dataUrl[(dataUrl.IndexOf(',') + 1)..]);
        var finished = clock.Elapsed.TotalMilliseconds;
        var nativeDecodeMs = finished - nativeDecodeStarted;
        var jsTotalMs = encoded.GetProperty("jsTotalMs").GetDouble();
        var pipelineMs = finished - pipelineStarted;
        return (new Timing(templateMs,
            encoded.GetProperty("decodeMs").GetDouble(), encoded.GetProperty("drawMs").GetDouble(),
            encoded.GetProperty("rotateMs").GetDouble(), encoded.GetProperty("encodeMs").GetDouble(),
            encoded.GetProperty("base64Ms").GetDouble(), nativeDecodeMs,
            pipelineMs - jsTotalMs - nativeDecodeMs, templateMs + pipelineMs, finished, bytes.Length), bytes);
    }

    private void ValidateImage(byte[] bytes)
    {
        using var stream = new MemoryStream(bytes);
        using var image = Image.FromStream(stream, useEmbeddedColorManagement: false, validateImageData: true);
        var width = options.Format == "png" ? 1920 : 462;
        var height = options.Format == "png" ? 462 : 1920;
        if (image.Width != width || image.Height != height)
            throw new InvalidOperationException($"Wrong dimensions: {image.Width}x{image.Height}, expected {width}x{height}");
    }

    private sealed record Timing(double TemplateMs, double DecodeMs, double DrawMs, double RotateMs,
        double EncodeMs, double Base64Ms, double NativeDecodeMs, double IpcAndDispatchMs,
        double TotalMs, double ObservedMs, int ImageBytes);
    private sealed record StyleResult(string Style, Timing FirstTiming, Dictionary<string, string[]> ImageHashes, List<Timing> Samples);
}
