package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"rsc.io/qr"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// ---------- Wiki ----------

func Wiki() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "usage: "+env.Cfg.Prefix+"wiki <query>")
		}
		q := strings.Join(env.Args, " ")
		u := "https://en.wikipedia.org/api/rest_v1/page/summary/" + url.PathEscape(strings.ReplaceAll(q, " ", "_"))
		b, err := httpGet(ctx, u)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "wiki: "+err.Error())
		}
		var r struct {
			Title     string `json:"title"`
			Extract   string `json:"extract"`
			ContentURLs struct {
				Desktop struct {
					Page string `json:"page"`
				} `json:"desktop"`
			} `json:"content_urls"`
		}
		if err := json.Unmarshal(b, &r); err != nil || r.Extract == "" {
			return sendText(ctx, env.Cli, env.Chat, "wiki: not found")
		}
		msg := fmt.Sprintf("*%s*\n\n%s", r.Title, r.Extract)
		if r.ContentURLs.Desktop.Page != "" {
			msg += "\n\n" + r.ContentURLs.Desktop.Page
		}
		return sendText(ctx, env.Cli, env.Chat, msg)
	}
}

// ---------- Define (dictionary) ----------

func Define() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "usage: "+env.Cfg.Prefix+"define <word>")
		}
		word := env.Args[0]
		b, err := httpGet(ctx, "https://api.dictionaryapi.dev/api/v2/entries/en/"+url.PathEscape(word))
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "define: "+err.Error())
		}
		var r []struct {
			Word     string `json:"word"`
			Meanings []struct {
				PartOfSpeech string `json:"partOfSpeech"`
				Definitions  []struct {
					Definition string `json:"definition"`
					Example    string `json:"example"`
				} `json:"definitions"`
			} `json:"meanings"`
		}
		if err := json.Unmarshal(b, &r); err != nil || len(r) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "no definition found")
		}
		var buf strings.Builder
		fmt.Fprintf(&buf, "*%s*\n", r[0].Word)
		for _, m := range r[0].Meanings {
			fmt.Fprintf(&buf, "\n_%s_\n", m.PartOfSpeech)
			for i, d := range m.Definitions {
				if i >= 2 {
					break
				}
				fmt.Fprintf(&buf, "• %s\n", d.Definition)
				if d.Example != "" {
					fmt.Fprintf(&buf, "  ex: %q\n", d.Example)
				}
			}
		}
		return sendText(ctx, env.Cli, env.Chat, buf.String())
	}
}

// ---------- Urban Dictionary ----------

func Urban() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "usage: "+env.Cfg.Prefix+"urban <term>")
		}
		term := strings.Join(env.Args, " ")
		b, err := httpGet(ctx, "https://api.urbandictionary.com/v0/define?term="+url.QueryEscape(term))
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "urban: "+err.Error())
		}
		var r struct {
			List []struct {
				Word       string `json:"word"`
				Definition string `json:"definition"`
				Example    string `json:"example"`
				ThumbsUp   int    `json:"thumbs_up"`
			} `json:"list"`
		}
		if err := json.Unmarshal(b, &r); err != nil || len(r.List) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "urban: no results")
		}
		d := r.List[0]
		clean := strings.NewReplacer("[", "", "]", "").Replace
		msg := fmt.Sprintf("*%s* (👍 %d)\n\n%s", d.Word, d.ThumbsUp, clean(d.Definition))
		if d.Example != "" {
			msg += "\n\n_ex:_ " + clean(d.Example)
		}
		return sendText(ctx, env.Cli, env.Chat, msg)
	}
}

// ---------- Joke ----------

func Joke() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		b, err := httpGet(ctx, "https://v2.jokeapi.dev/joke/Any?safe-mode")
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "joke: "+err.Error())
		}
		var r struct {
			Type     string `json:"type"`
			Joke     string `json:"joke"`
			Setup    string `json:"setup"`
			Delivery string `json:"delivery"`
			Error    bool   `json:"error"`
		}
		if err := json.Unmarshal(b, &r); err != nil || r.Error {
			return sendText(ctx, env.Cli, env.Chat, "joke: service error")
		}
		if r.Type == "single" {
			return sendText(ctx, env.Cli, env.Chat, r.Joke)
		}
		return sendText(ctx, env.Cli, env.Chat, r.Setup+"\n\n"+r.Delivery)
	}
}

// ---------- URL shortener ----------

func Short() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "usage: "+env.Cfg.Prefix+"short <url>")
		}
		longURL := env.Args[0]
		if !strings.HasPrefix(longURL, "http") {
			return sendText(ctx, env.Cli, env.Chat, "pass a full http(s) URL")
		}
		// is.gd — simple, no key
		b, err := httpGet(ctx, "https://is.gd/create.php?format=simple&url="+url.QueryEscape(longURL))
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "short: "+err.Error())
		}
		short := strings.TrimSpace(string(b))
		if !strings.HasPrefix(short, "http") {
			return sendText(ctx, env.Cli, env.Chat, "short: "+short)
		}
		return sendText(ctx, env.Cli, env.Chat, short)
	}
}

// ---------- QR code ----------

func QR() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "usage: "+env.Cfg.Prefix+"qr <text or url>")
		}
		text := strings.Join(env.Args, " ")
		code, err := qr.Encode(text, qr.M)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "qr: "+err.Error())
		}
		png := code.PNG()
		return uploadAndSendImage(ctx, env.Cli, env.Chat, png, "image/png")
	}
}

// ---------- Meme ----------

// Meme uses memegen.link — classic template format: /meme <template> <top text> | <bottom text>
// Template names like "doge", "drake", "spongebob", etc. See https://api.memegen.link/templates for list.
func Meme() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) < 2 {
			return sendText(ctx, env.Cli, env.Chat,
				"usage: "+env.Cfg.Prefix+"meme <template> <top> | <bottom>\ne.g. "+
					env.Cfg.Prefix+"meme drake old way | new way\ntemplates: drake, doge, bazinga, stonks, aag, buzz, fine, rollsafe, success")
		}
		template := env.Args[0]
		rest := strings.Join(env.Args[1:], " ")
		var top, bot string
		if i := strings.Index(rest, "|"); i >= 0 {
			top = strings.TrimSpace(rest[:i])
			bot = strings.TrimSpace(rest[i+1:])
		} else {
			top = rest
		}
		u := fmt.Sprintf("https://api.memegen.link/images/%s/%s/%s.jpg",
			url.PathEscape(template), memeText(top), memeText(bot))
		resp, err := httpCli.Get(u)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "meme: "+err.Error())
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("meme: http %d (bad template?)", resp.StatusCode))
		}
		img, _ := io.ReadAll(resp.Body)
		return uploadAndSendImage(ctx, env.Cli, env.Chat, img, "image/jpeg")
	}
}

// memeText escapes text per memegen's path conventions.
func memeText(s string) string {
	if s == "" {
		return "_"
	}
	r := strings.NewReplacer(
		" ", "_",
		"-", "--",
		"_", "__",
		"?", "~q",
		"&", "~a",
		"%", "~p",
		"#", "~h",
		"/", "~s",
		"\\", "~b",
		"<", "~l",
		">", "~g",
		"\"", "''",
	)
	return r.Replace(s)
}

// uploadAndSendImage is a shared helper to post an image to a chat.
func uploadAndSendImage(ctx context.Context, cli *whatsmeow.Client, chat types.JID, img []byte, mime string) error {
	up, err := cli.Upload(ctx, img, whatsmeow.MediaImage)
	if err != nil {
		return err
	}
	_, err = cli.SendMessage(ctx, chat, &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &up.URL,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			MediaKey:      up.MediaKey,
			Mimetype:      stringPtr(mime),
			DirectPath:    &up.DirectPath,
			FileLength:    uint64Ptr(uint64(len(img))),
		},
	})
	return err
}

// (unused; kept so http.StatusOK stays obviously available if needed later)
var _ = http.StatusOK
