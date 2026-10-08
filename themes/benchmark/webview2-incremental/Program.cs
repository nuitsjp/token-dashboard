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

internal sealed record Options(string OutputDirectory, string Mode, string Format, int Iterations, int Warmup)
{
    public int Width => Format == "jpeg" ? 462 : 1920;
    public int Height => Format == "jpeg" ? 1920 : 462;
    public static Options Parse(string[] args)
    {
        if (args.Length % 2 != 0)
            throw new ArgumentException("Usage: --out <directory> [--mode full|incremental] [--format png|jpeg] [--iterations 100] [--warmup 10]");
        var values = new Dictionary<string, string>();
        for (var i = 0; i < args.Length; i += 2)
            values.Add(args[i], args[i + 1]);
        if (!values.TryGetValue("--out", out var output))
            throw new ArgumentException("--out is required");
        var mode = values.GetValueOrDefault("--mode", "incremental");
        var format = values.GetValueOrDefault("--format", "png");
        var iterations = int.Parse(values.GetValueOrDefault("--iterations", "100"), CultureInfo.InvariantCulture);
        var warmup = int.Parse(values.GetValueOrDefault("--warmup", "10"), CultureInfo.InvariantCulture);
        if (mode is not ("full" or "incremental") || format is not ("png" or "jpeg") || iterations < 1 || warmup < 0)
            throw new ArgumentException("Invalid benchmark mode, format or sample count");
        return new Options(Path.GetFullPath(output), mode, format, iterations, warmup);
    }
}

internal sealed class BenchmarkForm : Form
{
    private const string BaseUrl = "http://127.0.0.1:9349";
    private readonly Options options;
    private readonly WebView2 view = new() { Dock = DockStyle.Fill };
    private readonly HttpClient http = new();
    private readonly JsonSerializerOptions jsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        WriteIndented = true,
    };
    private TaskCompletionSource<JsonElement>? pendingUpdate;
    private int messageId;

    public BenchmarkForm(Options options)
    {
        this.options = options;
        AutoScaleMode = AutoScaleMode.None;
        FormBorderStyle = FormBorderStyle.None;
        ClientSize = new Size(options.Width, options.Height);
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
            userDataFolder: Path.Combine(options.OutputDirectory, $"WebView2Profile-{options.Mode}-{options.Format}"));
        await view.EnsureCoreWebView2Async(environment);
        view.ZoomFactor = 1;
        var initializationMs = startup.Elapsed.TotalMilliseconds;
        view.CoreWebView2.WebMessageReceived += UpdateReceived;
        var styles = new List<StyleResult>();

        foreach (var id in new[] { "gauges", "bars" })
        {
            var style = id == "gauges" ? "Gauges" : "Bars";
            var firstStarted = Stopwatch.StartNew();
            await NavigateAsync($"{BaseUrl}/render?theme={id}&window=claude-session&remaining=72");
            var transform = options.Format == "jpeg" ? "transform-origin: 0 0; transform: translateX(462px) rotate(90deg);" : "";
            var geometry = await ScriptAsync($$"""
                const style = document.createElement('style');
                style.textContent = 'html,body { width: {{options.Width}}px; height: {{options.Height}}px; overflow: hidden; margin: 0; } main { {{transform}} }';
                document.head.appendChild(style);
                await document.fonts.ready;
                await Promise.all([...document.images].map(image => image.decode()));
                const root = document.querySelector('[data-window-id="claude-session"]');
                const fill = root.querySelector('[data-meter-fill]');
                let percent;
                if (root.tagName.toLowerCase() === 'g') {
                    const gauge = root.closest('.gauge');
                    const index = [...gauge.querySelectorAll('.window')].indexOf(root);
                    percent = gauge.querySelectorAll('.percentage-row strong')[index];
                } else {
                    percent = root.querySelector('.percent-value');
                }
                if (!fill || !percent) throw new Error('Missing meter or percentage node');
                const attribute = root.tagName.toLowerCase() === 'g' ? 'stroke-dasharray' : 'width';
                chrome.webview.addEventListener('message', event => {
                    const data = event.data;
                    try {
                        fill.setAttribute(attribute, attribute === 'width' ? `${data.remaining}%` : `${data.remaining} 100`);
                        percent.textContent = `${Math.round(data.remaining)}%`;
                        const tone = data.remaining < 25 ? 'danger' : data.remaining <= 40 ? 'warning' : 'normal';
                        fill.classList.remove('tone-normal', 'tone-warning', 'tone-danger');
                        fill.classList.add(`tone-${tone}`);
                        chrome.webview.postMessage({ updateId: data.updateId, ok: true,
                            result: { percent: percent.textContent, fill: fill.getAttribute(attribute), attribute } });
                    } catch (error) {
                        chrome.webview.postMessage({ updateId: data.updateId, ok: false, error: String(error) });
                    }
                });
                const rect = document.querySelector('main').getBoundingClientRect();
                if (rect.width !== {{options.Width}} || rect.height !== {{options.Height}} || devicePixelRatio !== 1)
                    throw new Error('Wrong geometry or pixel ratio');
                return { width: rect.width, height: rect.height, sourceWidth: 1920, sourceHeight: 462,
                    devicePixelRatio, icons: [...document.images].map(image => image.src),
                    retainedNodes: ['claude-session meter fill', 'claude-session percentage text'] };
                """);
            var firstImage = await CaptureAsync();
            var firstMs = firstStarted.Elapsed.TotalMilliseconds;
            ValidateImage(firstImage);
            await File.WriteAllBytesAsync(ImagePath(style, "first"), firstImage);

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
                if (!hashes.TryGetValue(remaining, out var values))
                    hashes[remaining] = values = [];
                values.Add(Convert.ToHexString(SHA256.HashData(image)));
            }
            if (hashes.TryGetValue(72, out var hashes72) && hashes.TryGetValue(71, out var hashes71) && hashes72.Overlaps(hashes71))
                throw new InvalidOperationException($"{style}: different remaining values produced the same image");
            foreach (var (remaining, image) in images)
                await File.WriteAllBytesAsync(ImagePath(style, remaining.ToString(CultureInfo.InvariantCulture)), image);
            styles.Add(new StyleResult(style, firstMs, geometry, hashes.ToDictionary(
                pair => pair.Key.ToString(CultureInfo.InvariantCulture), pair => pair.Value.ToArray()), samples));
            Console.WriteLine($"{style}: {samples.Count} {options.Mode}/{options.Format} captures validated");
        }

        var result = new
        {
            Method = $"webview2-{options.Mode}-{options.Format}",
            options.Mode,
            options.Format,
            RuntimeVersion = environment.BrowserVersionString,
            SdkVersion = "1.0.4258.31",
            options.Width,
            options.Height,
            options.Warmup,
            options.Iterations,
            InitializationMs = initializationMs,
            StopwatchFrequency = Stopwatch.Frequency,
            JpegQuality = options.Format == "jpeg" ? "WebView2 CapturePreviewAsync Jpeg default; public API does not expose quality control" : null,
            Rotation = options.Format == "jpeg" ? "Initial CSS transform rotates the 1920x462 theme 90 degrees clockwise into a 462x1920 WebView; capture directly encodes JPEG, with no image decode or re-encode" : "none",
            WindowMode = "Borderless visible parent HWND at (-10000, -10000), no activation, no browser flags. DPI unaware host, zoom 1, verified devicePixelRatio 1.",
            Boundary = options.Mode == "incremental"
                ? "totalMs = JSON serialization + PostWebMessageAsJson + mutation of retained SVG fill/text/tone nodes + WebMessageReceived acknowledgement + CapturePreviewAsync directly into a fresh MemoryStream. No HTTP per frame. First navigation/fonts/images, retained node discovery, validation, hashing, file writes and USB excluded. Initialization and first capture are separate. Samples alternate claude-session 72/71. No wait for requestAnimationFrame; capture completion is awaited and saved images must be checked for value correspondence."
                : "totalMs = server-side display-data preparation and Handlebars generation + full body HTML replacement/image decode/WebMessageReceived synchronization + CapturePreviewAsync directly into a fresh MemoryStream. Same main timing boundary as original full-update benchmark. HTTP excluded from totalMs, included in observedMs. Fonts/styles/icons cached. Validation, hashing, file writes and USB excluded. Initial CSS rotation remains in head across body replacements.",
            Styles = styles,
        };
        await File.WriteAllTextAsync(Path.Combine(options.OutputDirectory, $"webview2-{options.Mode}-{options.Format}.json"), JsonSerializer.Serialize(result, jsonOptions));
    }

    private string ImagePath(string style, string suffix) => Path.Combine(options.OutputDirectory,
        $"{style}-webview2-{options.Mode}-{suffix}.{(options.Format == "jpeg" ? "jpg" : "png")}");

    private void UpdateReceived(object? sender, CoreWebView2WebMessageReceivedEventArgs e)
    {
        using var message = JsonDocument.Parse(e.WebMessageAsJson);
        var root = message.RootElement;
        if (!root.TryGetProperty("updateId", out var id) || id.GetInt32() != messageId) return;
        if (root.GetProperty("ok").GetBoolean())
            pendingUpdate?.TrySetResult(root.GetProperty("result").Clone());
        else
            pendingUpdate?.TrySetException(new InvalidOperationException(root.GetProperty("error").GetString()));
    }

    private async Task NavigateAsync(string url)
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
            view.CoreWebView2.Navigate(url);
            await completion.Task.WaitAsync(TimeSpan.FromSeconds(30));
        }
        finally { view.CoreWebView2.NavigationCompleted -= Completed; }
    }

    // ExecuteScriptAsync does not await a JavaScript Promise: signal completion explicitly.
    private async Task<JsonElement> ScriptAsync(string script)
    {
        var id = Guid.NewGuid().ToString("N");
        var completion = new TaskCompletionSource<JsonElement>(TaskCreationOptions.RunContinuationsAsynchronously);
        void Received(object? sender, CoreWebView2WebMessageReceivedEventArgs e)
        {
            using var message = JsonDocument.Parse(e.WebMessageAsJson);
            var root = message.RootElement;
            if (!root.TryGetProperty("id", out var receivedId) || receivedId.GetString() != id) return;
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
        double htmlMs = 0;
        double domStarted = 0;
        JsonElement acknowledged = default;
        if (options.Mode == "incremental")
        {
            pendingUpdate = new TaskCompletionSource<JsonElement>(TaskCreationOptions.RunContinuationsAsynchronously);
            var json = JsonSerializer.Serialize(new { updateId = ++messageId, remaining });
            view.CoreWebView2.PostWebMessageAsJson(json);
            try { acknowledged = await pendingUpdate.Task.WaitAsync(TimeSpan.FromSeconds(30)); }
            finally { pendingUpdate = null; }
        }
        else
        {
            using var response = await http.GetAsync($"{BaseUrl}/render?theme={id}&window=claude-session&remaining={remaining}");
            response.EnsureSuccessStatusCode();
            var html = await response.Content.ReadAsStringAsync();
            htmlMs = double.Parse(response.Headers.GetValues("Server-Timing").Single().Split("dur=")[1], CultureInfo.InvariantCulture);
            var start = html.IndexOf("<body>", StringComparison.Ordinal) + 6;
            var markup = JsonSerializer.Serialize(html[start..html.LastIndexOf("</body>", StringComparison.Ordinal)]);
            domStarted = clock.Elapsed.TotalMilliseconds;
            await ScriptAsync($$"""
                document.body.innerHTML = {{markup}};
                await Promise.all([...document.images].map(image => image.decode()));
                return true;
                """);
        }
        var captureStarted = clock.Elapsed.TotalMilliseconds;
        var image = await CaptureAsync();
        var encoded = clock.Elapsed.TotalMilliseconds;
        if (options.Mode == "incremental")
        {
            var expectedFill = id == "gauges" ? $"{remaining} 100" : $"{remaining}%";
            if (acknowledged.GetProperty("percent").GetString() != $"{remaining}%" || acknowledged.GetProperty("fill").GetString() != expectedFill)
                throw new InvalidOperationException("The acknowledged DOM value did not match the requested value");
        }
        return (new Timing(remaining, htmlMs, captureStarted - domStarted, encoded - captureStarted,
            htmlMs + encoded - domStarted, encoded, image.Length), image);
    }

    private async Task<byte[]> CaptureAsync()
    {
        using var stream = new MemoryStream();
        var format = options.Format == "jpeg" ? CoreWebView2CapturePreviewImageFormat.Jpeg : CoreWebView2CapturePreviewImageFormat.Png;
        await view.CoreWebView2.CapturePreviewAsync(format, stream).WaitAsync(TimeSpan.FromSeconds(30));
        return stream.ToArray();
    }

    private void ValidateImage(byte[] encoded)
    {
        using var stream = new MemoryStream(encoded);
        using var image = Image.FromStream(stream);
        var expectedFormat = options.Format == "jpeg" ? System.Drawing.Imaging.ImageFormat.Jpeg : System.Drawing.Imaging.ImageFormat.Png;
        if (image.RawFormat.Guid != expectedFormat.Guid || image.Width != options.Width || image.Height != options.Height)
            throw new InvalidOperationException("CapturePreview returned an unexpected image format or dimensions");
    }

    private sealed record Timing(int Remaining, double HtmlMs, double DomMs, double CaptureMs, double TotalMs, double ObservedMs, int ImageBytes);
    private sealed record StyleResult(string Style, double FirstMs, JsonElement Geometry, Dictionary<string, string[]> ImageHashes, List<Timing> Samples);
}
