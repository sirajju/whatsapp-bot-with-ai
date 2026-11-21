// Package household exposes passive MQTT handlers that let devices on the
// home LAN (the wall-phone HA dashboard, buttons in rooms, scripts) nudge the
// bot — primarily to DM the owner "someone at home wants your attention."
//
// Today it's just the Pinger: subscribes to a single MQTT topic and, on every
// message, sends a short DM to the first OWNER. Rate-limited so one tap can't
// deliver 50 DMs if the subscriber flaps.
//
// Future: a Chime responder that publishes `intercom/play` with a canned
// chime URL so the wall phone emits an audible beep.
package household

import (
	"context"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"
)

// Subscriber is the slice of intercom.Bridge the Pinger needs — kept as an
// interface so tests and alternative transports (direct paho client, mocked
// broker) can plug in without importing the whole intercom package.
type Subscriber interface {
	Subscribe(topic string, qos byte, handler mqtt.MessageHandler) error
}

// NotifyFunc is "DM text to the first owner"; returns false (silently) when no
// owners are set or the WhatsApp send fails. Wired from main.go's notifyOwner.
type NotifyFunc func(text string) bool

type Pinger struct {
	log      zerolog.Logger
	notify   NotifyFunc
	cooldown time.Duration

	mu       sync.Mutex
	lastPing time.Time
}

// NewPinger subscribes to topic on sub and returns a running Pinger. The
// handler fires for every message on the topic. Content of the payload is
// ignored — the arrival IS the ping. Cooldown rejects follow-up messages
// within that window to prevent dashboard double-taps from spamming you.
func NewPinger(sub Subscriber, topic string, cooldown time.Duration, notify NotifyFunc, log zerolog.Logger) (*Pinger, error) {
	if cooldown == 0 {
		cooldown = 10 * time.Second
	}
	p := &Pinger{
		log:      log.With().Str("component", "household").Logger(),
		notify:   notify,
		cooldown: cooldown,
	}
	if err := sub.Subscribe(topic, 1, p.handle); err != nil {
		return nil, fmt.Errorf("subscribe %s: %w", topic, err)
	}
	p.log.Info().Str("topic", topic).Dur("cooldown", cooldown).Msg("household ping ready")
	return p, nil
}

func (p *Pinger) handle(_ mqtt.Client, msg mqtt.Message) {
	p.mu.Lock()
	if time.Since(p.lastPing) < p.cooldown {
		p.mu.Unlock()
		p.log.Debug().Str("topic", msg.Topic()).Msg("household ping suppressed (cooldown)")
		return
	}
	p.lastPing = time.Now()
	p.mu.Unlock()

	text := fmt.Sprintf("📞 *Someone at home is calling you* — %s", time.Now().Format("15:04 Mon"))
	if ok := p.notify(text); !ok {
		p.log.Warn().Msg("household ping: notify failed")
	} else {
		p.log.Info().Msg("household ping delivered")
	}
}

// ctx is accepted so the signature stays stable if we add retries / timers
// later; currently unused.
var _ = context.TODO
