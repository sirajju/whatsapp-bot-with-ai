// Package monitor watches system health (memory, disk, CPU load, CPU temp)
// and pings the configured alert sender whenever a metric crosses its
// threshold. Each metric tracks an alert/resolved state so one blip doesn't
// get spammed — and a cooldown keeps repeat alerts rare when a metric
// stays in trouble.
package monitor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
)

// SendFunc is how the monitor delivers alerts — typically "DM the first owner".
type SendFunc func(text string)

type Config struct {
	Enabled bool
	// How often to probe each metric.
	Interval time.Duration
	// Alert when available memory < this percent of total. Default 15.
	MemPct int
	// Alert when free disk on "/" < this percent of total. Default 10.
	DiskPct int
	// Alert when 1-min load average > runtime.NumCPU() * LoadMult. Default 2.0.
	LoadMult float64
	// Alert when CPU temp (°C) exceeds this. Default 75.
	TempC float64
	// Minimum gap between repeat alerts for the same metric. Default 30m.
	Cooldown time.Duration
}

type Monitor struct {
	cfg  Config
	log  zerolog.Logger
	send SendFunc

	state map[string]*metricState
}

type metricState struct {
	inAlert  bool
	lastSent time.Time
}

func New(cfg Config, log zerolog.Logger, send SendFunc) *Monitor {
	if cfg.Interval == 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.MemPct == 0 {
		cfg.MemPct = 15
	}
	if cfg.DiskPct == 0 {
		cfg.DiskPct = 10
	}
	if cfg.LoadMult == 0 {
		cfg.LoadMult = 2.0
	}
	if cfg.TempC == 0 {
		cfg.TempC = 75.0
	}
	if cfg.Cooldown == 0 {
		cfg.Cooldown = 30 * time.Minute
	}
	return &Monitor{
		cfg:   cfg,
		log:   log.With().Str("component", "monitor").Logger(),
		send:  send,
		state: make(map[string]*metricState),
	}
}

// Start runs the probe loop until ctx is cancelled. Safe to call when
// disabled — it logs and returns.
func (m *Monitor) Start(ctx context.Context) {
	if !m.cfg.Enabled {
		m.log.Info().Msg("system monitor disabled (MONITOR_ENABLED!=on)")
		return
	}
	m.log.Info().
		Dur("interval", m.cfg.Interval).
		Int("mem_pct", m.cfg.MemPct).
		Int("disk_pct", m.cfg.DiskPct).
		Float64("load_mult", m.cfg.LoadMult).
		Float64("temp_c", m.cfg.TempC).
		Msg("system monitor started")

	// Probe once immediately so a bad condition at startup is reported fast.
	m.probe()
	tick := time.NewTicker(m.cfg.Interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			m.probe()
		}
	}
}

func (m *Monitor) probe() {
	m.probeMem()
	m.probeDisk()
	m.probeLoad()
	m.probeTemp()
}

// --- metrics -------------------------------------------------------------

func (m *Monitor) probeMem() {
	total, avail, err := readMeminfo()
	if err != nil || total == 0 {
		return
	}
	pct := int(avail * 100 / total)
	if pct < m.cfg.MemPct {
		m.alert("mem",
			fmt.Sprintf("🧠 memory low — %d%% available (%d/%d MB)", pct, avail/1024, total/1024))
	} else {
		m.resolve("mem", fmt.Sprintf("✅ memory recovered — %d%% available", pct))
	}
}

func (m *Monitor) probeDisk() {
	free, total, err := readDisk("/")
	if err != nil || total == 0 {
		return
	}
	pct := int(free * 100 / total)
	if pct < m.cfg.DiskPct {
		m.alert("disk",
			fmt.Sprintf("💾 disk low on / — %d%% free (%d/%d GB)", pct, free>>30, total>>30))
	} else {
		m.resolve("disk", fmt.Sprintf("✅ disk recovered — %d%% free on /", pct))
	}
}

func (m *Monitor) probeLoad() {
	load1, err := readLoadAvg()
	if err != nil {
		return
	}
	ncpu := float64(runtime.NumCPU())
	threshold := ncpu * m.cfg.LoadMult
	if load1 > threshold {
		m.alert("load",
			fmt.Sprintf("⚙️ CPU busy — load1 %.2f > %.0f (ncpu %d × %.1f)", load1, threshold, int(ncpu), m.cfg.LoadMult))
	} else {
		m.resolve("load", fmt.Sprintf("✅ CPU recovered — load1 %.2f", load1))
	}
}

func (m *Monitor) probeTemp() {
	t, err := readCPUTemp()
	if err != nil {
		return
	}
	if t > m.cfg.TempC {
		m.alert("temp",
			fmt.Sprintf("🔥 CPU hot — %.1f°C (threshold %.0f°C)", t, m.cfg.TempC))
	} else {
		m.resolve("temp", fmt.Sprintf("✅ CPU cool — %.1f°C", t))
	}
}

// --- state transitions --------------------------------------------------

func (m *Monitor) alert(key, text string) {
	s := m.getState(key)
	if s.inAlert {
		// Already in alert — only re-ping if cooldown elapsed.
		if time.Since(s.lastSent) >= m.cfg.Cooldown {
			m.log.Warn().Str("metric", key).Msg("repeat alert (cooldown elapsed)")
			m.send(text)
			s.lastSent = time.Now()
		}
		return
	}
	// Fresh alert.
	m.log.Warn().Str("metric", key).Msg("alert")
	s.inAlert = true
	s.lastSent = time.Now()
	m.send(text)
}

func (m *Monitor) resolve(key, text string) {
	s := m.getState(key)
	if !s.inAlert {
		return
	}
	m.log.Info().Str("metric", key).Msg("resolved")
	s.inAlert = false
	m.send(text)
}

func (m *Monitor) getState(key string) *metricState {
	if s, ok := m.state[key]; ok {
		return s
	}
	s := &metricState{}
	m.state[key] = s
	return s
}

// --- probes --------------------------------------------------------------

// readMeminfo parses /proc/meminfo and returns (MemTotal, MemAvailable) in kB.
func readMeminfo() (int64, int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	var total, avail int64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseInt(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			avail = v
		}
	}
	if err := sc.Err(); err != nil {
		return 0, 0, err
	}
	return total, avail, nil
}

// readDisk returns (freeBytes, totalBytes) for the filesystem containing path.
func readDisk(path string) (int64, int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	total := int64(st.Blocks) * st.Bsize
	free := int64(st.Bavail) * st.Bsize
	return free, total, nil
}

func readLoadAvg() (float64, error) {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(b))
	if len(fields) < 1 {
		return 0, fmt.Errorf("bad /proc/loadavg format")
	}
	return strconv.ParseFloat(fields[0], 64)
}

// readCPUTemp tries thermal_zone0 first, then falls back to looking for any
// zone named "x86_pkg_temp" / "coretemp" / "cpu_thermal". Returns °C.
func readCPUTemp() (float64, error) {
	// Fast path.
	if t, err := readZoneTemp("/sys/class/thermal/thermal_zone0/temp"); err == nil {
		return t, nil
	}
	// Scan all zones, pick a CPU-like one.
	entries, err := os.ReadDir("/sys/class/thermal")
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "thermal_zone") {
			continue
		}
		base := "/sys/class/thermal/" + e.Name()
		typ, _ := os.ReadFile(base + "/type")
		t := strings.TrimSpace(string(typ))
		if t == "x86_pkg_temp" || t == "coretemp" || t == "cpu_thermal" || strings.Contains(t, "cpu") {
			if v, err := readZoneTemp(base + "/temp"); err == nil {
				return v, nil
			}
		}
	}
	return 0, fmt.Errorf("no thermal zone found")
}

func readZoneTemp(path string) (float64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, err
	}
	return float64(v) / 1000.0, nil
}
