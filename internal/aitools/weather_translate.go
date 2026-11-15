package aitools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/ai"
)

var hc = &http.Client{Timeout: 15 * time.Second}

func getWeatherTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name:        "get_weather",
			Description: "Current weather for a named city (any country). Returns temp, humidity, wind, conditions.",
			Parameters: objectSchema(map[string]any{
				"city": stringProp("City name — natural language, e.g. 'Kochi' or 'San Francisco'."),
			}, "city"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			city, err := requireString(args, "city")
			if err != nil {
				return "", err
			}
			return weatherText(env.Ctx, city)
		},
	}
}

func translateTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name:        "translate",
			Description: "Translate text into a target language. Returns the translated text.",
			Parameters: objectSchema(map[string]any{
				"text":          stringProp("The text to translate."),
				"target_lang":   stringProp("ISO 639-1 target language code, e.g. 'es', 'fr', 'ja', 'ml'."),
				"source_lang":   stringProp("Optional ISO 639-1 source language; defaults to 'en' if unsure."),
			}, "text", "target_lang"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			text, err := requireString(args, "text")
			if err != nil {
				return "", err
			}
			target, err := requireString(args, "target_lang")
			if err != nil {
				return "", err
			}
			src := optString(args, "source_lang")
			if src == "" {
				src = "en"
			}
			u := fmt.Sprintf(
				"https://api.mymemory.translated.net/get?q=%s&langpair=%s|%s",
				url.QueryEscape(text), url.QueryEscape(src), url.QueryEscape(target))
			b, err := getBody(env.Ctx, u)
			if err != nil {
				return "", err
			}
			var r struct {
				ResponseData struct {
					TranslatedText string `json:"translatedText"`
				} `json:"responseData"`
				ResponseStatus int `json:"responseStatus"`
			}
			if err := json.Unmarshal(b, &r); err != nil {
				return "", err
			}
			if r.ResponseStatus != 200 || r.ResponseData.TranslatedText == "" {
				return "", fmt.Errorf("translate: empty response (check language codes)")
			}
			return r.ResponseData.TranslatedText, nil
		},
	}
}

func weatherText(ctx context.Context, city string) (string, error) {
	// Open-Meteo: free, no key.
	geoURL := "https://geocoding-api.open-meteo.com/v1/search?count=1&name=" + url.QueryEscape(city)
	gb, err := getBody(ctx, geoURL)
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("city not found: %s", city)
	}
	g := geo.Results[0]
	wURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m,relative_humidity_2m,wind_speed_10m,weather_code",
		g.Latitude, g.Longitude)
	wb, err := getBody(ctx, wURL)
	if err != nil {
		return "", err
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
		return "", err
	}
	return strings.TrimSpace(fmt.Sprintf(
		"%s, %s — %.1f°C, %d%% humidity, %.1f km/h wind (code %d)",
		g.Name, g.Country, w.Current.Temp, w.Current.Humidity, w.Current.Wind, w.Current.Code,
	)), nil
}

func getBody(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wabot/1.0 (+https://github.com/sirajju/whatsapp-bot-with-ai)")
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256*1024))
}
