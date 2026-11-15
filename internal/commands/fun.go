package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

var httpCli = &http.Client{Timeout: 20 * time.Second}

// EightBall returns one of the classic answers.
func EightBall() router.Handler {
	answers := []string{
		"it is certain", "without a doubt", "yes definitely", "you may rely on it",
		"ask again later", "cannot predict now", "concentrate and ask again",
		"don't count on it", "my reply is no", "very doubtful", "outlook not so good",
	}
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "ask a yes/no question after !8ball")
		}
		return sendText(ctx, env.Cli, env.Chat, "🎱 "+answers[rand.Intn(len(answers))])
	}
}

// Roll rolls an NdM dice expression; default 1d6.
func Roll() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		expr := "1d6"
		if len(env.Args) > 0 {
			expr = env.Args[0]
		}
		parts := strings.SplitN(strings.ToLower(expr), "d", 2)
		if len(parts) != 2 {
			return sendText(ctx, env.Cli, env.Chat, "usage: !roll 2d20")
		}
		n, errN := strconv.Atoi(parts[0])
		m, errM := strconv.Atoi(parts[1])
		if errN != nil || errM != nil || n <= 0 || m <= 1 || n > 100 {
			return sendText(ctx, env.Cli, env.Chat, "bad dice: NdM with 1<=N<=100, M>=2")
		}
		rolls := make([]string, n)
		total := 0
		for i := 0; i < n; i++ {
			r := rand.Intn(m) + 1
			total += r
			rolls[i] = strconv.Itoa(r)
		}
		return sendText(ctx, env.Cli, env.Chat,
			fmt.Sprintf("🎲 %s = %d  (%s)", expr, total, strings.Join(rolls, ", ")))
	}
}

// Calc evaluates a simple arithmetic expression (+ - * / parens, int/float).
func Calc() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "usage: !calc <expr>   e.g. !calc 2*(3+4)")
		}
		expr := strings.Join(env.Args, " ")
		v, err := evalExpr(expr)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "calc: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("%s = %g", expr, v))
	}
}

// Translate uses MyMemory's free public translation API (no key, 5000 words/day).
// Accepts either /tr <target> <text>           — assumes English source
//       or     /tr <src>-<target> <text>       — explicit, e.g. /tr ja-en こんにちは
func Translate() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) < 2 {
			return sendText(ctx, env.Cli, env.Chat,
				"usage: "+env.Cfg.Prefix+"tr <target> <text>   or   "+env.Cfg.Prefix+"tr <src>-<target> <text>")
		}
		src, target := "en", strings.ToLower(env.Args[0])
		if i := strings.Index(target, "-"); i > 0 {
			src, target = target[:i], target[i+1:]
		}
		text := strings.Join(env.Args[1:], " ")

		u := fmt.Sprintf(
			"https://api.mymemory.translated.net/get?q=%s&langpair=%s|%s",
			url.QueryEscape(text), url.QueryEscape(src), url.QueryEscape(target))
		b, err := httpGet(ctx, u)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "translate: "+err.Error())
		}
		var r struct {
			ResponseData struct {
				TranslatedText string `json:"translatedText"`
			} `json:"responseData"`
			ResponseStatus    int    `json:"responseStatus"`
			ResponseDetails   string `json:"responseDetails"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "translate parse: "+err.Error())
		}
		if r.ResponseStatus != 200 || r.ResponseData.TranslatedText == "" {
			msg := r.ResponseDetails
			if msg == "" {
				msg = "empty (check lang codes)"
			}
			return sendText(ctx, env.Cli, env.Chat, "translate: "+msg)
		}
		return sendText(ctx, env.Cli, env.Chat, r.ResponseData.TranslatedText)
	}
}

// Weather uses Open-Meteo (free, no key). Arg = city name.
func Weather() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "usage: !weather <city>")
		}
		city := strings.Join(env.Args, " ")

		geoURL := "https://geocoding-api.open-meteo.com/v1/search?count=1&name=" + url.QueryEscape(city)
		gb, err := httpGet(ctx, geoURL)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "geo: "+err.Error())
		}
		var geo struct {
			Results []struct {
				Name      string  `json:"name"`
				Country   string  `json:"country"`
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"results"`
		}
		if err := json.Unmarshal(gb, &geo); err != nil || len(geo.Results) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "city not found")
		}
		g := geo.Results[0]

		wURL := fmt.Sprintf(
			"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m,relative_humidity_2m,wind_speed_10m,weather_code",
			g.Latitude, g.Longitude)
		wb, err := httpGet(ctx, wURL)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "weather: "+err.Error())
		}
		var w struct {
			Current struct {
				Temp     float64 `json:"temperature_2m"`
				Humidity int     `json:"relative_humidity_2m"`
				Wind     float64 `json:"wind_speed_10m"`
				Code     int     `json:"weather_code"`
			} `json:"current"`
		}
		if err := json.Unmarshal(wb, &w); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "weather parse: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf(
			"*%s, %s*\n%s\n🌡 %.1f°C\n💧 %d%% humidity\n💨 %.1f km/h wind",
			g.Name, g.Country, weatherCode(w.Current.Code),
			w.Current.Temp, w.Current.Humidity, w.Current.Wind))
	}
}

// Quote fetches a random one from ZenQuotes (free, no key — quotable.io shut down in 2024).
func Quote() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		b, err := httpGet(ctx, "https://zenquotes.io/api/random")
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "quote: "+err.Error())
		}
		var r []struct {
			Q string `json:"q"`
			A string `json:"a"`
		}
		if err := json.Unmarshal(b, &r); err != nil || len(r) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "quote parse: empty")
		}
		return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("“%s”\n— %s", r[0].Q, r[0].A))
	}
}

// Lyrics uses lyrics.ovh (free, no key). Args: artist - title OR just title.
func Lyrics() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "usage: !lyrics <artist> - <title>")
		}
		joined := strings.Join(env.Args, " ")
		var artist, title string
		if i := strings.Index(joined, "-"); i >= 0 {
			artist = strings.TrimSpace(joined[:i])
			title = strings.TrimSpace(joined[i+1:])
		} else {
			artist, title = "", joined
		}
		u := fmt.Sprintf("https://api.lyrics.ovh/v1/%s/%s",
			url.PathEscape(artist), url.PathEscape(title))
		b, err := httpGet(ctx, u)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "lyrics: "+err.Error())
		}
		var r struct {
			Lyrics string `json:"lyrics"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal(b, &r); err != nil || r.Error != "" || r.Lyrics == "" {
			return sendText(ctx, env.Cli, env.Chat, "lyrics not found")
		}
		if len(r.Lyrics) > 3500 {
			r.Lyrics = r.Lyrics[:3500] + "..."
		}
		return sendText(ctx, env.Cli, env.Chat, r.Lyrics)
	}
}

// ---------- helpers ----------

// userAgent identifies wabot to upstream services. Wikipedia and a few other
// public APIs 403 clients that don't set a specific UA.
const userAgent = "wabot/1.0 (+https://github.com/sirajju/whatsapp-bot-with-ai)"

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, */*")
	resp, err := httpCli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func weatherCode(c int) string {
	switch {
	case c == 0:
		return "☀️ clear"
	case c <= 3:
		return "⛅ partly cloudy"
	case c <= 48:
		return "🌫 foggy"
	case c <= 67:
		return "🌧 rain"
	case c <= 77:
		return "❄️ snow"
	case c <= 82:
		return "🌦 showers"
	case c <= 99:
		return "⛈ thunderstorm"
	}
	return "unknown"
}
