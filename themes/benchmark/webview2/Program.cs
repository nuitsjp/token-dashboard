using System.Buffers.Binary;
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
        if (args.Length != 1)
            throw new ArgumentException("Usage: WebView2Benchmark.exe <absolute output directory>");

        Application.SetHighDpiMode(HighDpiMode.DpiUnaware);
        Application.EnableVisualStyles();
        using var form = new BenchmarkForm(Path.GetFullPath(args[0]));
        Application.Run(form);
    }
}

internal sealed class BenchmarkForm : Form
{
    private const string BaseUrl = "http://127.0.0.1:9349";
    private const int Warmup = 10;
    private const int Iterations = 100;
    private readonly string outputDirectory;
    private readonly WebView2 view = new() { Dock = DockStyle.Fill };
    private readonly HttpClient http = new();
    private readonly JsonSerializerOptions jsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        WriteIndented = true,
    };

    public BenchmarkForm(string outputDirectory)
    {
        this.outputDirectory = outputDirectory;
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
        try
        {
            await RunAsync();
        }
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
        Directory.CreateDirectory(outputDirectory);
        var startup = Stopwatch.StartNew();
        var environment = await CoreWebView2Environment.CreateAsync(
            userDataFolder: Path.Combine(outputDirectory, "WebView2Profile"));
        await view.EnsureCoreWebView2Async(environment);
        view.ZoomFactor = 1;
        var initializationMs = startup.Elapsed.TotalMilliseconds;
        var userAgent = JsonSerializer.Deserialize<string>(await view.CoreWebView2.ExecuteScriptAsync("navigator.userAgent"));
        var styles = new List<StyleResult>();

        foreach (var id in new[] { "gauges", "bars" })
        {
            var style = id == "gauges" ? "Gauges" : "Bars";
            var firstStarted = Stopwatch.StartNew();
            await NavigateAsync($"{BaseUrl}/render?theme={id}&window=claude-session&remaining=72");
            var geometry = await ScriptAsync("""
                await document.fonts.ready;
                await Promise.all([...document.images].map(image => image.decode()));
                const rect = document.querySelector('main').getBoundingClientRect();
                if (rect.width !== 1920 || rect.height !== 462 || devicePixelRatio !== 1)
                    throw new Error('Wrong geometry or pixel ratio');
                return { width: rect.width, height: rect.height, devicePixelRatio,
                    icons: [...document.images].map(image => image.src) };
                """);
            var firstPng = await CaptureAsync();
            var firstMs = firstStarted.Elapsed.TotalMilliseconds;
            ValidatePng(firstPng);
            await File.WriteAllBytesAsync(Path.Combine(outputDirectory, $"{style}-webview2-first.png"), firstPng);

            for (var i = 0; i < Warmup; i++)
                await MeasureAsync(id, 72 - i % 2);

            var samples = new List<Timing>();
            var images = new Dictionary<int, byte[]>();
            var hashes = new Dictionary<int, HashSet<string>>();
            for (var i = 0; i < Iterations; i++)
            {
                var remaining = 72 - i % 2;
                var (timing, png) = await MeasureAsync(id, remaining);
                samples.Add(timing);
                ValidatePng(png);
                images[remaining] = png;
                if (!hashes.TryGetValue(remaining, out var values))
                    hashes[remaining] = values = [];
                values.Add(Convert.ToHexString(SHA256.HashData(png)));
            }
            if (hashes[72].Overlaps(hashes[71]))
                throw new InvalidOperationException($"{style}: different remaining values produced the same PNG");
            foreach (var (remaining, png) in images)
                await File.WriteAllBytesAsync(Path.Combine(outputDirectory, $"{style}-webview2-{remaining}.png"), png);
            styles.Add(new StyleResult(style, firstMs, geometry, hashes.ToDictionary(
                pair => pair.Key.ToString(CultureInfo.InvariantCulture), pair => pair.Value.ToArray()), samples));
            Console.WriteLine($"{style}: {samples.Count} captures validated");
        }

        var result = new
        {
            Method = "handlebars-webview2-capturepreview",
            RuntimeVersion = environment.BrowserVersionString,
            SdkVersion = "1.0.4258.31",
            UserAgent = userAgent,
            Width = 1920,
            Height = 462,
            Warmup,
            Iterations,
            InitializationMs = initializationMs,
            StopwatchFrequency = Stopwatch.Frequency,
            WindowMode = "Borderless, visible parent HWND outside the desktop at (-10000, -10000), without activation. No browser flags. DPI unaware host, zoom 1, verified devicePixelRatio 1.",
            Boundary = "totalMs = server-side display-data preparation and Handlebars generation + DOM replacement/image decoding/WebMessageReceived synchronization + native CoreWebView2.CapturePreviewAsync(Png) into a fresh MemoryStream. HTTP transport is excluded from totalMs and included in observedMs. Samples alternate claude-session remaining percent between 72 and 71. Fonts/styles/icons are cached. Fixture loading/conversion, validation, hashing, file writes and USB transfer are excluded. firstMs measures first navigation/fonts/images/PNG capture for each theme, excluding WebView2 initialization, which is reported separately.",
            Styles = styles,
        };
        await File.WriteAllTextAsync(Path.Combine(outputDirectory, "webview2.json"), JsonSerializer.Serialize(result, jsonOptions));
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
        finally
        {
            view.CoreWebView2.NavigationCompleted -= Completed;
        }
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
            if (root.GetProperty("id").GetString() != id) return;
            if (root.GetProperty("ok").GetBoolean())
                completion.TrySetResult(root.GetProperty("result").Clone());
            else
                completion.TrySetException(new InvalidOperationException(root.GetProperty("error").GetString()));
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
        finally
        {
            view.CoreWebView2.WebMessageReceived -= Received;
        }
    }

    private async Task<(Timing, byte[])> MeasureAsync(string id, int remaining)
    {
        var clock = Stopwatch.StartNew();
        using var response = await http.GetAsync($"{BaseUrl}/render?theme={id}&window=claude-session&remaining={remaining}");
        response.EnsureSuccessStatusCode();
        var html = await response.Content.ReadAsStringAsync();
        var htmlMs = double.Parse(response.Headers.GetValues("Server-Timing").Single().Split("dur=")[1], CultureInfo.InvariantCulture);
        var start = html.IndexOf("<body>", StringComparison.Ordinal) + 6;
        var body = html[start..html.LastIndexOf("</body>", StringComparison.Ordinal)];
        var markup = JsonSerializer.Serialize(body);
        var domStarted = clock.Elapsed.TotalMilliseconds;
        await ScriptAsync($$"""
            document.body.innerHTML = {{markup}};
            await Promise.all([...document.images].map(image => image.decode()));
            return true;
            """);
        var captureStarted = clock.Elapsed.TotalMilliseconds;
        var png = await CaptureAsync();
        var encoded = clock.Elapsed.TotalMilliseconds;
        return (new Timing(htmlMs, captureStarted - domStarted, encoded - captureStarted,
            htmlMs + encoded - domStarted, encoded, png.Length), png);
    }

    private async Task<byte[]> CaptureAsync()
    {
        using var stream = new MemoryStream();
        await view.CoreWebView2.CapturePreviewAsync(CoreWebView2CapturePreviewImageFormat.Png, stream)
            .WaitAsync(TimeSpan.FromSeconds(30));
        return stream.ToArray();
    }

    private static void ValidatePng(byte[] png)
    {
        if (!png.AsSpan(0, 8).SequenceEqual(new byte[] { 137, 80, 78, 71, 13, 10, 26, 10 }) ||
            BinaryPrimitives.ReadInt32BigEndian(png.AsSpan(16, 4)) != 1920 ||
            BinaryPrimitives.ReadInt32BigEndian(png.AsSpan(20, 4)) != 462)
            throw new InvalidOperationException("CapturePreview returned an invalid PNG or wrong dimensions");
    }

    private sealed record Timing(double HtmlMs, double DomMs, double CaptureMs, double TotalMs, double ObservedMs, int PngBytes);
    private sealed record StyleResult(string Style, double FirstMs, JsonElement Geometry, Dictionary<string, string[]> PngHashes, List<Timing> Samples);
}
