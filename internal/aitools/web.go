package aitools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/ai"
)

func webSearchTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name: "web_search",
			Description: "Search the public web. Returns up to 5 result titles + URLs + short snippets. " +
				"Use when you need current information the model doesn't know.",
			Parameters: objectSchema(map[string]any{
				"query": stringProp("Search query — natural language is fine."),
			}, "query"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			q, err := requireString(args, "query")
			if err != nil {
				return "", err
			}
			results, err := ddgSearch(env.Ctx, q)
			if err != nil {
				return "", err
			}
			if len(results) == 0 {
				return "No results.", nil
			}
			var b strings.Builder
			for i, r := range results {
				fmt.Fprintf(&b, "%d. %s\n   %s\n   %s\n", i+1, r.Title, r.URL, r.Snippet)
			}
			return strings.TrimSpace(b.String()), nil
		},
	}
}

type ddgResult struct {
	Title, URL, Snippet string
}

var (
	ddgHTTP = &http.Client{Timeout: 15 * time.Second}

	// DDG HTML links are like: <a class="result__a" href="/l/?uddg=<encoded-url>">Title</a>
	ddgLinkRe    = regexp.MustCompile(`(?s)<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	ddgSnippetRe = regexp.MustCompile(`(?s)<a[^>]*class="result__snippet"[^>]*>(.*?)</a>`)
	tagRe        = regexp.MustCompile(`<[^>]+>`)
)

// ddgSearch scrapes DuckDuckGo's HTML endpoint (no API key, no quota).
func ddgSearch(ctx context.Context, q string) ([]ddgResult, error) {
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://html.duckduckgo.com/html/",
		strings.NewReader(url.Values{"q": {q}}.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "wabot/1.0 (+https://github.com/sirajju/whatsapp-bot-with-ai)")
	resp, err := ddgHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, err
	}

	hm := ddgLinkRe.FindAllStringSubmatch(string(body), -1)
	sm := ddgSnippetRe.FindAllStringSubmatch(string(body), -1)

	out := make([]ddgResult, 0, 5)
	for i, m := range hm {
		if len(out) >= 5 {
			break
		}
		link := m[1]
		title := strings.TrimSpace(tagRe.ReplaceAllString(m[2], ""))
		if u, err := unwrapDDG(link); err == nil {
			link = u
		}
		snippet := ""
		if i < len(sm) {
			snippet = strings.TrimSpace(tagRe.ReplaceAllString(sm[i][1], ""))
		}
		if title == "" {
			continue
		}
		out = append(out, ddgResult{Title: title, URL: link, Snippet: snippet})
	}
	return out, nil
}

// unwrapDDG converts "/l/?uddg=https%3A..." → the decoded real URL.
func unwrapDDG(raw string) (string, error) {
	if !strings.Contains(raw, "uddg=") {
		return raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw, err
	}
	if v := u.Query().Get("uddg"); v != "" {
		if d, err := url.QueryUnescape(v); err == nil {
			return d, nil
		}
	}
	return raw, nil
}
