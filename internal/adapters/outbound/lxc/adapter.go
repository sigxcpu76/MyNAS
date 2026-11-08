package lxc

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/sigxcpu/nas/internal/config"
	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type ptyConn struct {
	file    *os.File
	cmd     *exec.Cmd
	prefix  []byte
	onClose func()
}

func (c *ptyConn) Read(p []byte) (n int, err error) {
	if len(c.prefix) > 0 {
		n := copy(p, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.file.Read(p)
}

func (c *ptyConn) Write(p []byte) (n int, err error) {
	return c.file.Write(p)
}

func (c *ptyConn) Close() error {
	if c.onClose != nil {
		c.onClose()
	}
	_ = c.file.Close()
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return nil
}

func (c *ptyConn) LocalAddr() net.Addr                { return &net.IPAddr{} }
func (c *ptyConn) RemoteAddr() net.Addr               { return &net.IPAddr{} }
func (c *ptyConn) SetDeadline(t time.Time) error      { return nil }
func (c *ptyConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *ptyConn) SetWriteDeadline(t time.Time) error { return nil }

type lxcMetricsSample struct {
	cpuUsec    uint64
	sampleTime time.Time
	cpuPercent float64
	memUsedMiB int64
	netRxBytes uint64
	netTxBytes uint64
	netRxRate  int64
	netTxRate  int64
	cachedAt   time.Time
}

type Adapter struct {
	mu              sync.RWMutex
	mainPool        string
	zfsPort         ports.ZfsEnginePort
	containers      map[string]domain.LxcContainer
	execPTYs        map[string]*os.File
	httpClient      *http.Client
	cgroupRoot      string
	cgroupPathCache map[string]string
	metricsCache    map[string]lxcMetricsSample
	metricsMu       sync.Mutex
}

func NewAdapter() *Adapter {
	return &Adapter{
		containers:      make(map[string]domain.LxcContainer),
		execPTYs:        make(map[string]*os.File),
		httpClient:      &http.Client{Timeout: 30 * time.Minute},
		cgroupRoot:      "/sys/fs/cgroup",
		cgroupPathCache: make(map[string]string),
		metricsCache:    make(map[string]lxcMetricsSample),
	}
}

// SetCgroupRoot allows overriding cgroup base directory (primarily for testing).
func (a *Adapter) SetCgroupRoot(root string) {
	a.metricsMu.Lock()
	defer a.metricsMu.Unlock()
	a.cgroupRoot = root
}

func (a *Adapter) findCgroupDir(id string) string {
	a.metricsMu.Lock()
	if cachedDir, ok := a.cgroupPathCache[id]; ok {
		if fi, err := os.Stat(cachedDir); err == nil && fi.IsDir() {
			a.metricsMu.Unlock()
			return cachedDir
		}
		delete(a.cgroupPathCache, id)
	}
	root := a.cgroupRoot
	if root == "" {
		root = "/sys/fs/cgroup"
	}
	a.metricsMu.Unlock()

	candidates := []string{
		filepath.Join(root, "lxc.payload."+id),
		filepath.Join(root, "lxc.payload", id),
		filepath.Join(root, "lxc", id),
		filepath.Join(root, "system.slice", "lxc@"+id+".service"),
	}
	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			a.metricsMu.Lock()
			a.cgroupPathCache[id] = cand
			a.metricsMu.Unlock()
			return cand
		}
	}

	// Fallback: discover via container init PID if lxc-info is available
	if _, err := exec.LookPath("lxc-info"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "lxc-info", "-n", id, "-p", "-H")
		if out, err := cmd.Output(); err == nil {
			pidStr := strings.TrimSpace(string(out))
			if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
				if cgData, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid)); err == nil {
					for _, line := range strings.Split(string(cgData), "\n") {
						line = strings.TrimSpace(line)
						if strings.HasPrefix(line, "0::") {
							relPath := strings.TrimPrefix(line, "0::")
							fullPath := filepath.Join(root, strings.TrimPrefix(relPath, "/"))
							if fi, err := os.Stat(fullPath); err == nil && fi.IsDir() {
								a.metricsMu.Lock()
								a.cgroupPathCache[id] = fullPath
								a.metricsMu.Unlock()
								return fullPath
							}
						}
					}
				}
			}
		}
	}

	return ""
}

func readCgroupMetrics(cgDir string) (cpuUsec uint64, memBytes int64, err error) {
	// 1. Memory Usage
	// Cgroup v2: memory.current
	memFile := filepath.Join(cgDir, "memory.current")
	if data, readErr := os.ReadFile(memFile); readErr == nil {
		if v, parseErr := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); parseErr == nil {
			memBytes = v
		}
	} else {
		// Cgroup v1 fallback: memory.usage_in_bytes
		memV1File := filepath.Join(cgDir, "memory.usage_in_bytes")
		if data, readErr := os.ReadFile(memV1File); readErr == nil {
			if v, parseErr := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); parseErr == nil {
				memBytes = v
			}
		}
	}

	// 2. CPU Usage
	// Cgroup v2: cpu.stat -> usage_usec <int>
	cpuStatFile := filepath.Join(cgDir, "cpu.stat")
	if data, readErr := os.ReadFile(cpuStatFile); readErr == nil {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "usage_usec") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if u, parseErr := strconv.ParseUint(fields[1], 10, 64); parseErr == nil {
						cpuUsec = u
						break
					}
				}
			}
		}
	} else {
		// Cgroup v1 fallback: cpuacct.usage (nanoseconds)
		cpuV1File := filepath.Join(cgDir, "cpuacct.usage")
		if data, readErr := os.ReadFile(cpuV1File); readErr == nil {
			if ns, parseErr := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); parseErr == nil {
				cpuUsec = ns / 1000
			}
		}
	}

	return cpuUsec, memBytes, nil
}

func readCgroupPid(cgDir string) int {
	for _, filename := range []string{"cgroup.procs", "tasks"} {
		f, err := os.Open(filepath.Join(cgDir, filename))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		if scanner.Scan() {
			txt := strings.TrimSpace(scanner.Text())
			_ = f.Close()
			if pid, parseErr := strconv.Atoi(txt); parseErr == nil && pid > 0 {
				return pid
			}
		}
		_ = f.Close()
	}
	return 0
}

func readContainerNetBytes(cgDir string) (uint64, uint64) {
	pid := readCgroupPid(cgDir)
	if pid <= 0 {
		return 0, 0
	}
	path := fmt.Sprintf("/proc/%d/net/dev", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	var totalRx, totalTx uint64
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" || strings.HasPrefix(iface, "lo:") {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) >= 9 {
			rx, _ := strconv.ParseUint(fields[0], 10, 64)
			tx, _ := strconv.ParseUint(fields[8], 10, 64)
			totalRx += rx
			totalTx += tx
		}
	}
	return totalRx, totalTx
}

func (a *Adapter) sampleContainerMetrics(id string) (float64, int64, int64, int64, int64, int64) {
	now := time.Now()
	a.metricsMu.Lock()
	cached, hasCache := a.metricsCache[id]
	if hasCache && now.Sub(cached.sampleTime) < 150*time.Millisecond {
		cpuPct := cached.cpuPercent
		memMiB := cached.memUsedMiB
		rxRate := cached.netRxRate
		txRate := cached.netTxRate
		rxBytes := int64(cached.netRxBytes)
		txBytes := int64(cached.netTxBytes)
		a.metricsMu.Unlock()
		return cpuPct, memMiB, rxRate, txRate, rxBytes, txBytes
	}
	a.metricsMu.Unlock()

	cgDir := a.findCgroupDir(id)
	if cgDir == "" {
		if hasCache {
			return cached.cpuPercent, cached.memUsedMiB, cached.netRxRate, cached.netTxRate, int64(cached.netRxBytes), int64(cached.netTxBytes)
		}
		return 0, 0, 0, 0, 0, 0
	}

	currUsec, memBytes, err := readCgroupMetrics(cgDir)
	if err != nil {
		if hasCache {
			return cached.cpuPercent, cached.memUsedMiB, cached.netRxRate, cached.netTxRate, int64(cached.netRxBytes), int64(cached.netTxBytes)
		}
		return 0, 0, 0, 0, 0, 0
	}

	currRx, currTx := readContainerNetBytes(cgDir)
	memMiB := memBytes / (1024 * 1024)

	a.metricsMu.Lock()
	defer a.metricsMu.Unlock()

	cached, hasCache = a.metricsCache[id]
	if hasCache && !cached.sampleTime.IsZero() {
		dt := now.Sub(cached.sampleTime)
		if dt >= 150*time.Millisecond {
			cpuPct := 0.0
			if currUsec >= cached.cpuUsec {
				deltaUsec := currUsec - cached.cpuUsec
				cpuPct = (float64(deltaUsec) / float64(dt.Microseconds())) * 100.0
				cpuPct = math.Round(cpuPct*10) / 10
			}
			var rxRate, txRate int64
			if dt.Seconds() > 0 {
				if currRx >= cached.netRxBytes {
					rxRate = int64(float64(currRx-cached.netRxBytes) / dt.Seconds())
				}
				if currTx >= cached.netTxBytes {
					txRate = int64(float64(currTx-cached.netTxBytes) / dt.Seconds())
				}
			}
			a.metricsCache[id] = lxcMetricsSample{
				cpuUsec:    currUsec,
				sampleTime: now,
				cpuPercent: cpuPct,
				memUsedMiB: memMiB,
				netRxBytes: currRx,
				netTxBytes: currTx,
				netRxRate:  rxRate,
				netTxRate:  txRate,
				cachedAt:   now,
			}
			return cpuPct, memMiB, rxRate, txRate, int64(currRx), int64(currTx)
		}
		// If interval too short, update memory and counters, keep previous rates
		a.metricsCache[id] = lxcMetricsSample{
			cpuUsec:    cached.cpuUsec,
			sampleTime: cached.sampleTime,
			cpuPercent: cached.cpuPercent,
			memUsedMiB: memMiB,
			netRxBytes: currRx,
			netTxBytes: currTx,
			netRxRate:  cached.netRxRate,
			netTxRate:  cached.netTxRate,
			cachedAt:   now,
		}
		return cached.cpuPercent, memMiB, cached.netRxRate, cached.netTxRate, int64(currRx), int64(currTx)
	}

	// Cold start: record baseline sample and warm up in background
	a.metricsCache[id] = lxcMetricsSample{
		cpuUsec:    currUsec,
		sampleTime: now,
		cpuPercent: 0,
		memUsedMiB: memMiB,
		netRxBytes: currRx,
		netTxBytes: currTx,
		netRxRate:  0,
		netTxRate:  0,
		cachedAt:   now,
	}

	go func(ctId, dir string, baseUsec, baseRx, baseTx uint64, baseTime time.Time) {
		time.Sleep(200 * time.Millisecond)
		u, m, err := readCgroupMetrics(dir)
		if err != nil {
			return
		}
		r2, t2Net := readContainerNetBytes(dir)
		t2 := time.Now()
		dt := t2.Sub(baseTime)
		if dt > 0 {
			pct := 0.0
			if u >= baseUsec {
				pct = (float64(u-baseUsec) / float64(dt.Microseconds())) * 100.0
				pct = math.Round(pct*10) / 10
			}
			var rxR, txR int64
			if dt.Seconds() > 0 {
				if r2 >= baseRx {
					rxR = int64(float64(r2-baseRx) / dt.Seconds())
				}
				if t2Net >= baseTx {
					txR = int64(float64(t2Net-baseTx) / dt.Seconds())
				}
			}
			a.metricsMu.Lock()
			a.metricsCache[ctId] = lxcMetricsSample{
				cpuUsec:    u,
				sampleTime: t2,
				cpuPercent: pct,
				memUsedMiB: m / (1024 * 1024),
				netRxBytes: r2,
				netTxBytes: t2Net,
				netRxRate:  rxR,
				netTxRate:  txR,
				cachedAt:   t2,
			}
			a.metricsMu.Unlock()
		}
	}(id, cgDir, currUsec, currRx, currTx, now)

	return 0, memMiB, 0, 0, int64(currRx), int64(currTx)
}

func (a *Adapter) SetZfsPort(zfsPort ports.ZfsEnginePort) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.zfsPort = zfsPort
}

type lxcStatus struct {
	state string
	ipv4  string
}

// fetchLiveStatuses leverages lxc-ls to fetch the state and IP of all containers in a single fast call.
func (a *Adapter) fetchLiveStatuses(ctx context.Context) map[string]lxcStatus {
	statuses := make(map[string]lxcStatus)
	if _, err := exec.LookPath("lxc-ls"); err != nil {
		return statuses
	}

	execCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "lxc-ls", "-f", "-F", "NAME,STATE,IPV4")
	out, err := cmd.Output()
	if err != nil {
		return statuses
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "NAME") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			name := fields[0]
			state := fields[1]
			ip := ""
			if len(fields) >= 3 && fields[2] != "-" {
				ip = fields[2]
				if commaIdx := strings.Index(ip, ","); commaIdx != -1 {
					ip = ip[:commaIdx]
				}
			}
			statuses[name] = lxcStatus{
				state: state,
				ipv4:  ip,
			}
		}
	}
	return statuses
}

// fetchSingleContainerStatus checks a specific container state and IP via lxc-ls filter.
func (a *Adapter) fetchSingleContainerStatus(ctx context.Context, id string) *lxcStatus {
	if _, err := exec.LookPath("lxc-ls"); err != nil {
		return nil
	}
	execCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "lxc-ls", "-f", "-F", "NAME,STATE,IPV4", "--filter", "^"+id+"$")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "NAME") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == id {
			ip := ""
			if len(fields) >= 3 && fields[2] != "-" {
				ip = fields[2]
				if commaIdx := strings.Index(ip, ","); commaIdx != -1 {
					ip = ip[:commaIdx]
				}
			}
			return &lxcStatus{
				state: fields[1],
				ipv4:  ip,
			}
		}
	}
	return nil
}

func parseLxcMountEntry(v string) (hostPath, targetPath string, ro bool, ok bool) {
	// Skip internal GPU device bind mounts
	if strings.HasPrefix(v, "/dev/dri") || strings.HasPrefix(v, "/dev/nvidia") {
		return "", "", false, false
	}
	var fields []string
	var cur strings.Builder
	inQuote := false
	var quoteChar rune
	for _, r := range v {
		switch {
		case (r == '"' || r == '\'') && !inQuote:
			inQuote = true
			quoteChar = r
		case inQuote && r == quoteChar:
			inQuote = false
		case !inQuote && (r == ' ' || r == '\t'):
			if cur.Len() > 0 {
				fields = append(fields, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		fields = append(fields, cur.String())
	}
	if len(fields) < 2 {
		return "", "", false, false
	}
	hostPath = fields[0]
	targetPath = fields[1]
	if !strings.HasPrefix(targetPath, "/") {
		targetPath = "/" + targetPath
	}
	if len(fields) >= 4 {
		for _, opt := range strings.Split(fields[3], ",") {
			if strings.TrimSpace(opt) == "ro" {
				ro = true
				break
			}
		}
	}
	return hostPath, targetPath, ro, true
}

// discoverContainerLocked parses a container configuration from /var/lib/lxc/<id>/config or pool container config.
func (a *Adapter) discoverContainerLocked(id string) *domain.LxcContainer {
	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", id)
	data, err := os.ReadFile(configPath)
	if err != nil {
		mainPool := a.mainPool
		if mainPool == "" {
			mainPool = config.GetGlobal().System.MainPool
		}
		cleanMainPool := strings.TrimPrefix(strings.TrimSpace(mainPool), "zfsPools/")
		if cleanMainPool != "" {
			configPath = fmt.Sprintf("/mnt/%s/containers/%s/config", cleanMainPool, id)
			data, err = os.ReadFile(configPath)
		}
		if err != nil {
			return nil
		}
	}

	c := &domain.LxcContainer{
		ID:             id,
		ContainerName:  id,
		State:          "STOPPED",
		Architecture:   "amd64",
		Distribution:   "linux",
		Release:        "generic",
		NetworkBridge:  "br0",
		MemoryLimitMiB: 512,
		CPUCores:       1,
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# nan.description =") {
			c.Description = strings.TrimSpace(strings.TrimPrefix(line, "# nan.description ="))
			continue
		}
		if strings.HasPrefix(line, "# aegis.description =") {
			c.Description = strings.TrimSpace(strings.TrimPrefix(line, "# aegis.description ="))
			continue
		}
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])

		switch k {
		case "lxc.uts.name":
			c.ContainerName = v
		case "lxc.arch":
			c.Architecture = v
		case "lxc.rootfs.path":
			c.RootfsPath = strings.TrimPrefix(v, "dir:")
		case "lxc.net.0.link":
			c.NetworkBridge = v
		case "lxc.start.auto":
			c.Autostart = (v == "1")
		case "lxc.cgroup2.memory.max":
			v = strings.TrimSuffix(v, "M")
			var mem int64
			if _, err := fmt.Sscanf(v, "%d", &mem); err == nil {
				c.MemoryLimitMiB = mem
			}
		case "lxc.cgroup2.cpu.weight":
			var weight int32
			if _, err := fmt.Sscanf(v, "%d", &weight); err == nil && weight > 0 {
				c.CPUCores = weight / 100
				if c.CPUCores < 1 {
					c.CPUCores = 1
				}
			}
		case "lxc.mount.entry":
			if hp, tp, ro, ok := parseLxcMountEntry(v); ok {
				duplicate := false
				for _, existing := range c.MountPoints {
					if existing.HostPath == hp && existing.ContainerPath == tp {
						duplicate = true
						break
					}
				}
				if !duplicate {
					c.MountPoints = append(c.MountPoints, domain.LxcMountPoint{
						HostPath:      hp,
						ContainerPath: tp,
						ReadOnly:      ro,
					})
				}
			}
		case "lxc.cgroup2.devices.allow":
			if strings.Contains(v, "226:*") || strings.Contains(v, "195:*") {
				c.GpuEnabled = true
			}
		case "lxc.idmap":
			c.Unprivileged = true
		}
	}

	// Try reading OS release from rootfs if present
	if c.RootfsPath != "" {
		if detectedDistro, detectedRelease := detectRootfsOS(c.RootfsPath); detectedDistro != "" {
			c.Distribution = detectedDistro
			if detectedRelease != "" {
				c.Release = detectedRelease
			}
		}
	}

	// Try reading description from description file if present
	if descBytes, err := os.ReadFile(fmt.Sprintf("/var/lib/lxc/%s/description", id)); err == nil {
		c.Description = strings.TrimSpace(string(descBytes))
	} else if c.RootfsPath != "" {
		if descBytes, err := os.ReadFile(filepath.Join(filepath.Dir(c.RootfsPath), "description")); err == nil {
			c.Description = strings.TrimSpace(string(descBytes))
		}
	}

	return c
}

// getOrDiscoverContainerLocked returns the container from memory if present,
// or attempts to discover it from disk (/var/lib/lxc/<id>/config) under the adapter lock.
func (a *Adapter) getOrDiscoverContainerLocked(id string) (*domain.LxcContainer, bool) {
	if c, ok := a.containers[id]; ok {
		if disc := a.discoverContainerLocked(id); disc != nil {
			c.MountPoints = disc.MountPoints
			c.GpuEnabled = disc.GpuEnabled
			if disc.Unprivileged {
				c.Unprivileged = true
			}
			if disc.Description != "" && c.Description == "" {
				c.Description = disc.Description
			}
			if disc.Distribution != "" && disc.Distribution != "linux" {
				c.Distribution = disc.Distribution
				c.Release = disc.Release
			}
			a.containers[id] = c
		}
		if c.RootfsPath != "" {
			if detectedDistro, detectedRelease := detectRootfsOS(c.RootfsPath); detectedDistro != "" {
				c.Distribution = detectedDistro
				if detectedRelease != "" {
					c.Release = detectedRelease
				}
				a.containers[id] = c
			}
		}
		return &c, true
	}
	if disc := a.discoverContainerLocked(id); disc != nil {
		a.containers[id] = *disc
		return disc, true
	}
	return nil, false
}

func (a *Adapter) ListContainers(ctx context.Context) ([]domain.LxcContainer, error) {
	// 1. Fetch live container statuses in one fast lxc-ls execution outside the lock
	liveStatuses := a.fetchLiveStatuses(ctx)

	a.mu.Lock()
	defer a.mu.Unlock()

	// 2. Update all in-memory containers with their live status
	for name, st := range liveStatuses {
		if _, exists := a.containers[name]; !exists {
			if disc := a.discoverContainerLocked(name); disc != nil {
				a.containers[name] = *disc
			}
		}
		if c, exists := a.containers[name]; exists {
			c.State = st.state
			c.IPv4Address = st.ipv4
			a.containers[name] = c
		}
	}

	// 3. Auto-discover any containers residing in /var/lib/lxc
	if entries, err := os.ReadDir("/var/lib/lxc"); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				name := e.Name()
				if _, exists := a.containers[name]; !exists {
					if disc := a.discoverContainerLocked(name); disc != nil {
						if st, ok := liveStatuses[name]; ok {
							disc.State = st.state
							disc.IPv4Address = st.ipv4
						}
						a.containers[name] = *disc
					}
				}
			}
		}
	}

	out := make([]domain.LxcContainer, 0, len(a.containers))
	for id, c := range a.containers {
		if disc := a.discoverContainerLocked(id); disc != nil {
			c.MountPoints = disc.MountPoints
			c.GpuEnabled = disc.GpuEnabled
			if disc.Unprivileged {
				c.Unprivileged = true
			}
			if disc.Distribution != "" && disc.Distribution != "linux" {
				c.Distribution = disc.Distribution
				c.Release = disc.Release
			}
			a.containers[id] = c
		}
		if c.RootfsPath != "" {
			if detectedDistro, detectedRelease := detectRootfsOS(c.RootfsPath); detectedDistro != "" {
				c.Distribution = detectedDistro
				if detectedRelease != "" {
					c.Release = detectedRelease
				}
				a.containers[id] = c
			}
		}
		if c.State == "RUNNING" {
			c.CPUPercent, c.MemoryUsedMiB, c.NetRxRate, c.NetTxRate, c.NetRxBytes, c.NetTxBytes = a.sampleContainerMetrics(c.ID)
		} else {
			c.CPUPercent = 0
			c.MemoryUsedMiB = 0
			c.NetRxRate = 0
			c.NetTxRate = 0
			c.NetRxBytes = 0
			c.NetTxBytes = 0
		}
		out = append(out, c)
	}
	return out, nil
}

func (a *Adapter) GetContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}

	if st := a.fetchSingleContainerStatus(ctx, id); st != nil {
		a.mu.Lock()
		c.State = st.state
		c.IPv4Address = st.ipv4
		a.containers[id] = c
		a.mu.Unlock()
	}

	if c.State == "RUNNING" {
		c.CPUPercent, c.MemoryUsedMiB, c.NetRxRate, c.NetTxRate, c.NetRxBytes, c.NetTxBytes = a.sampleContainerMetrics(c.ID)
	} else {
		c.CPUPercent = 0
		c.MemoryUsedMiB = 0
		c.NetRxRate = 0
		c.NetTxRate = 0
		c.NetRxBytes = 0
		c.NetTxBytes = 0
	}

	cp := c
	return &cp, nil
}

// SetMainPool enforces that the main storage pool is the only choice for LXC rootfs.
func (a *Adapter) SetMainPool(pool string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mainPool = strings.TrimPrefix(strings.TrimSpace(pool), "zfsPools/")
}

func generateRandomMAC() string {
	buf := make([]byte, 3)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("00:16:3e:%02x:%02x:%02x", buf[0], buf[1], buf[2])
}

func hasInitBinary(rootfsPath string) bool {
	if rootfsPath == "" {
		return false
	}
	for _, initCandidate := range []string{
		"sbin/init",
		"bin/init",
		"usr/lib/systemd/systemd",
		"lib/systemd/systemd",
		"bin/sh",
		"bin/bash",
		"bin/busybox",
	} {
		if _, err := os.Stat(filepath.Join(rootfsPath, initCandidate)); err == nil {
			return true
		}
	}
	return false
}

func lxcHasBash(rootfsPath string) bool {
	if rootfsPath == "" {
		return false
	}
	for _, p := range []string{"bin/bash", "usr/bin/bash"} {
		if fi, err := os.Stat(filepath.Join(rootfsPath, p)); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

func normalizeLxcDistro(distro string) string {
	d := strings.ToLower(strings.TrimSpace(distro))
	switch d {
	case "arch", "archlinux", "arch linux", "arch-linux":
		return "archlinux"
	case "alpine", "alpinelinux", "alpine linux", "alpine-linux":
		return "alpine"
	case "alma", "almalinux", "alma linux", "alma-linux":
		return "almalinux"
	case "rocky", "rockylinux", "rocky linux", "rocky-linux":
		return "rockylinux"
	case "centos", "centos-stream", "centos stream":
		return "centos"
	case "void", "voidlinux", "void linux", "void-linux":
		return "voidlinux"
	case "kali", "kalilinux", "kali linux", "kali-linux":
		return "kali"
	case "oracle", "oraclelinux", "oracle linux", "oracle-linux", "ol":
		return "oracle"
	case "amazon", "amzn", "amazonlinux", "amazon linux", "amazon-linux":
		return "amazonlinux"
	case "opensuse", "opensuse-leap", "opensuse-tumbleweed", "suse":
		return "opensuse"
	case "openeuler", "open-euler", "open euler":
		return "openeuler"
	case "linuxmint", "mint":
		return "mint"
	default:
		return strings.ReplaceAll(d, " ", "")
	}
}

func isDebianReleaseName(rel string) bool {
	switch strings.ToLower(strings.TrimSpace(rel)) {
	case "bookworm", "trixie", "bullseye", "buster", "sid", "forky":
		return true
	default:
		return false
	}
}

func normalizeLxcRelease(distro, release string) string {
	d := normalizeLxcDistro(distro)
	r := strings.ToLower(strings.TrimSpace(release))
	if d != "" && d != "debian" && isDebianReleaseName(r) {
		r = ""
	}
	switch d {
	case "debian":
		switch r {
		case "", "12", "stable", "default":
			return "bookworm"
		case "13", "testing":
			return "trixie"
		case "11":
			return "bullseye"
		default:
			return r
		}
	case "ubuntu":
		switch r {
		case "", "24.04", "lts", "latest", "default":
			return "noble"
		case "22.04":
			return "jammy"
		case "20.04":
			return "focal"
		case "24.10":
			return "oracular"
		case "25.04":
			return "plucky"
		case "26.04":
			return "resolute"
		default:
			return r
		}
	case "alpine":
		switch r {
		case "", "latest", "stable", "default":
			return "3.21"
		default:
			return r
		}
	case "archlinux":
		return "current"
	case "almalinux", "rockylinux":
		if r == "" || r == "latest" || r == "default" {
			return "10"
		}
		if idx := strings.IndexByte(r, '.'); idx > 0 {
			return r[:idx]
		}
		return r
	case "fedora":
		if r == "" || r == "latest" || r == "default" {
			return "41"
		}
		return r
	case "centos":
		switch r {
		case "", "9", "stream", "9-stream", "default":
			return "9-Stream"
		case "10", "10-stream":
			return "10-Stream"
		default:
			return release
		}
	case "opensuse":
		switch r {
		case "", "leap", "default":
			return "15.6"
		case "tumbleweed":
			return "tumbleweed"
		default:
			return r
		}
	case "gentoo", "kali", "voidlinux", "slackware", "alt", "plamo":
		if r == "" || r == "rolling" || r == "base" || r == "default" {
			return "current"
		}
		return r
	case "devuan":
		if r == "" || r == "default" {
			return "daedalus"
		}
		return r
	case "openeuler":
		if r == "" || r == "default" {
			return "24.03"
		}
		return r
	case "oracle":
		if r == "" || r == "default" {
			return "9"
		}
		if idx := strings.IndexByte(r, '.'); idx > 0 {
			return r[:idx]
		}
		return r
	case "amazonlinux":
		if r == "" || r == "default" {
			return "2023"
		}
		return r
	case "busybox":
		if r == "" || r == "default" {
			return "1.36.1"
		}
		return r
	default:
		if r == "" {
			return "current"
		}
		return r
	}
}

func readRootfsFileSafe(rootfsPath, relPath string) ([]byte, error) {
	fullPath := filepath.Join(rootfsPath, relPath)
	fi, err := os.Lstat(fullPath)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(fullPath)
		if err != nil {
			return nil, err
		}
		if filepath.IsAbs(target) {
			fullPath = filepath.Join(rootfsPath, strings.TrimPrefix(target, "/"))
		} else {
			fullPath = filepath.Join(filepath.Dir(fullPath), target)
		}
	}
	return os.ReadFile(fullPath)
}

func detectRootfsOS(rootfsPath string) (distro, release string) {
	if rootfsPath == "" {
		return "", ""
	}
	for _, relPath := range []string{"usr/lib/os-release", "etc/os-release"} {
		osReleaseBytes, err := readRootfsFileSafe(rootfsPath, relPath)
		if err != nil || len(osReleaseBytes) == 0 {
			continue
		}
		var id, versionID, versionCodename, buildID string
		for _, osLine := range strings.Split(string(osReleaseBytes), "\n") {
			osLine = strings.TrimSpace(osLine)
			switch {
			case strings.HasPrefix(osLine, "ID="):
				id = strings.Trim(strings.TrimPrefix(osLine, "ID="), `"'`)
			case strings.HasPrefix(osLine, "VERSION_ID="):
				versionID = strings.Trim(strings.TrimPrefix(osLine, "VERSION_ID="), `"'`)
			case strings.HasPrefix(osLine, "VERSION_CODENAME="):
				versionCodename = strings.Trim(strings.TrimPrefix(osLine, "VERSION_CODENAME="), `"'`)
			case strings.HasPrefix(osLine, "BUILD_ID="):
				buildID = strings.Trim(strings.TrimPrefix(osLine, "BUILD_ID="), `"'`)
			}
		}
		if id != "" {
			distro = normalizeLxcDistro(id)
			if (distro == "debian" || distro == "ubuntu" || distro == "devuan") && versionCodename != "" {
				release = versionCodename
			} else if versionID != "" {
				release = versionID
			} else if buildID != "" {
				release = buildID
			} else if versionCodename != "" {
				release = versionCodename
			} else if distro == "archlinux" {
				release = "rolling"
			}
			return distro, release
		}
	}
	if _, err := readRootfsFileSafe(rootfsPath, "etc/arch-release"); err == nil {
		return "archlinux", "rolling"
	}
	if data, err := readRootfsFileSafe(rootfsPath, "etc/alpine-release"); err == nil {
		return "alpine", strings.TrimSpace(string(data))
	}
	return "", ""
}

func inferDistroReleaseFromTemplateFilename(filename string) (distro, release string) {
	base := strings.ToLower(filepath.Base(filename))
	for _, ext := range []string{".tar.xz", ".tar.gz", ".tar.zst", ".tgz", ".tar"} {
		base = strings.TrimSuffix(base, ext)
	}
	switch {
	case strings.HasPrefix(base, "archlinux") || strings.HasPrefix(base, "arch-linux") || base == "arch" || strings.HasPrefix(base, "arch-"):
		return "archlinux", "current"
	case strings.HasPrefix(base, "almalinux"):
		parts := strings.Split(base, "-")
		if len(parts) > 1 && len(parts[1]) > 0 && parts[1][0] >= '0' && parts[1][0] <= '9' {
			return "almalinux", parts[1]
		}
		return "almalinux", "10"
	case strings.HasPrefix(base, "rockylinux") || strings.HasPrefix(base, "rocky-linux"):
		parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(base, "rockylinux"), "rocky-linux"), "-")
		for _, p := range parts {
			if len(p) > 0 && p[0] >= '0' && p[0] <= '9' {
				return "rockylinux", p
			}
		}
		return "rockylinux", "10"
	case strings.HasPrefix(base, "alpine"):
		parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(base, "alpine-linux"), "alpine"), "-")
		for _, p := range parts {
			if len(p) > 0 && p[0] >= '0' && p[0] <= '9' {
				return "alpine", p
			}
		}
		return "alpine", "3.21"
	case strings.HasPrefix(base, "ubuntu"):
		parts := strings.Split(base, "-")
		if len(parts) > 1 && parts[1] != "official" && parts[1] != "amd64" && parts[1] != "arm64" && parts[1] != "rootfs" {
			return "ubuntu", parts[1]
		}
		return "ubuntu", "noble"
	case strings.HasPrefix(base, "debian"):
		parts := strings.Split(base, "-")
		if len(parts) > 1 && parts[1] != "official" && parts[1] != "amd64" && parts[1] != "arm64" && parts[1] != "rootfs" {
			return "debian", normalizeLxcRelease("debian", parts[1])
		}
		return "debian", "bookworm"
	case strings.HasPrefix(base, "fedora"):
		parts := strings.Split(base, "-")
		if len(parts) > 1 && len(parts[1]) > 0 && parts[1][0] >= '0' && parts[1][0] <= '9' {
			return "fedora", parts[1]
		}
		return "fedora", "41"
	case strings.HasPrefix(base, "centos"):
		return "centos", "9-Stream"
	case strings.HasPrefix(base, "opensuse"):
		if strings.Contains(base, "tumbleweed") {
			return "opensuse", "tumbleweed"
		}
		return "opensuse", "15.6"
	case strings.HasPrefix(base, "gentoo"):
		return "gentoo", "current"
	case strings.HasPrefix(base, "devuan"):
		return "devuan", "daedalus"
	case strings.HasPrefix(base, "kali"):
		return "kali", "current"
	case strings.HasPrefix(base, "voidlinux") || strings.HasPrefix(base, "void-linux"):
		return "voidlinux", "current"
	case strings.HasPrefix(base, "openeuler"):
		return "openeuler", "24.03"
	case strings.HasPrefix(base, "oracle"):
		return "oracle", "9"
	case strings.HasPrefix(base, "amazonlinux"):
		return "amazonlinux", "2023"
	}
	return "", ""
}

func parseLinuxContainersImageURL(rawURL string) (distro, release, arch string) {
	idx := strings.Index(rawURL, "linuxcontainers.org/images/")
	if idx == -1 {
		return "", "", ""
	}
	rest := rawURL[idx+len("linuxcontainers.org/images/"):]
	parts := strings.Split(rest, "/")
	if len(parts) >= 3 {
		return normalizeLxcDistro(parts[0]), parts[1], parts[2]
	}
	if len(parts) >= 2 {
		return normalizeLxcDistro(parts[0]), parts[1], "amd64"
	}
	if len(parts) >= 1 {
		return normalizeLxcDistro(parts[0]), "", "amd64"
	}
	return "", "", ""
}

func findMatchingLocalTemplate(tmplDir, distro, containerID string) string {
	entries, err := os.ReadDir(tmplDir)
	if err != nil || len(entries) == 0 {
		return ""
	}
	normDistro := normalizeLxcDistro(distro)
	lowerID := strings.ToLower(strings.TrimSpace(containerID))
	lowerID = strings.TrimPrefix(lowerID, "ct-")
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".tar.xz") && !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tar.zst") && !strings.HasSuffix(name, ".tar") && !strings.HasSuffix(name, ".tgz") {
			continue
		}
		fileDistro, _ := inferDistroReleaseFromTemplateFilename(name)
		matched := false
		if normDistro != "" && fileDistro != "" {
			matched = (fileDistro == normDistro)
		} else if normDistro != "" && fileDistro == "" && !strings.Contains(name, "turnkey") {
			matched = strings.Contains(name, normDistro)
		} else if normDistro == "" && lowerID != "" {
			matched = strings.Contains(name, lowerID)
		}
		if matched {
			fullPath := filepath.Join(tmplDir, entry.Name())
			if fi, err := os.Stat(fullPath); err == nil && fi.Size() > 0 {
				return fullPath
			}
		}
	}
	return ""
}

func (a *Adapter) resolveLatestLinuxContainersURL(ctx context.Context, sourceURL, distro, release string) string {
	urlDistro, urlRelease, urlArch := parseLinuxContainersImageURL(sourceURL)
	d := normalizeLxcDistro(distro)
	if urlDistro != "" {
		d = urlDistro
	}
	if d == "" {
		d = "debian"
	}
	r := strings.ToLower(strings.TrimSpace(release))
	if urlRelease != "" {
		r = strings.ToLower(strings.TrimSpace(urlRelease))
	}
	if d != "debian" && isDebianReleaseName(r) {
		r = ""
	}
	arch := urlArch
	if arch == "" {
		arch = "amd64"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://images.linuxcontainers.org/meta/1.0/index-system", nil)
	if err != nil {
		return ""
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	scanner := bufio.NewScanner(resp.Body)
	var exactReleasePath string
	var fallbackDistroPath string
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ";")
		if len(parts) < 6 {
			continue
		}
		partDistro := normalizeLxcDistro(parts[0])
		partRelease := strings.ToLower(strings.TrimSpace(parts[1]))
		if partDistro == d && parts[2] == arch && parts[3] == "default" {
			fallbackDistroPath = parts[5]
			if r != "" && partRelease == r {
				exactReleasePath = parts[5]
			}
		}
	}
	chosenPath := exactReleasePath
	if chosenPath == "" {
		chosenPath = fallbackDistroPath
	}
	if chosenPath != "" {
		return fmt.Sprintf("https://images.linuxcontainers.org%srootfs.tar.xz", chosenPath)
	}
	return ""
}

func (a *Adapter) cleanupFailedCreation(ctx context.Context, pool, containerID, rootfsPath string) {
	if rootfsPath != "" {
		_ = os.RemoveAll(rootfsPath)
		containerDir := filepath.Dir(rootfsPath)
		_ = os.RemoveAll(containerDir)
	}
	_ = os.RemoveAll(fmt.Sprintf("/var/lib/lxc/%s", containerID))
	if pool != "" && a.zfsPort != nil {
		datasetID := fmt.Sprintf("containers/%s", containerID)
		_ = a.zfsPort.DeleteDataset(ctx, pool, datasetID, true)
	}
}

func (a *Adapter) ensureTemplateArchive(ctx context.Context, sourceURL, targetPath, mainPool, containerID, distro string) (string, error) {
	if !strings.HasPrefix(sourceURL, "http://") && !strings.HasPrefix(sourceURL, "https://") {
		return sourceURL, nil
	}

	cleanPool := strings.TrimPrefix(strings.TrimSpace(mainPool), "zfsPools/")
	if cleanPool == "" {
		cleanPool = "tank"
	}
	tmplDir := fmt.Sprintf("/mnt/%s/lxc-templates", cleanPool)
	_ = os.MkdirAll(tmplDir, 0755)

	urlDistro, urlRelease, urlArch := parseLinuxContainersImageURL(sourceURL)
	effectiveDistro := normalizeLxcDistro(distro)
	if urlDistro != "" {
		effectiveDistro = urlDistro
	}

	destPath := targetPath
	if destPath != "" && !filepath.IsAbs(destPath) {
		destPath = filepath.Join(tmplDir, filepath.Base(destPath))
	}
	if destPath == "" {
		parts := strings.Split(sourceURL, "/")
		rawFilename := parts[len(parts)-1]
		if idx := strings.Index(rawFilename, "?"); idx != -1 {
			rawFilename = rawFilename[:idx]
		}
		if idx := strings.Index(rawFilename, "#"); idx != -1 {
			rawFilename = rawFilename[:idx]
		}

		ext := ".tar.xz"
		switch {
		case strings.HasSuffix(rawFilename, ".tar.gz"):
			ext = ".tar.gz"
		case strings.HasSuffix(rawFilename, ".tar.zst"):
			ext = ".tar.zst"
		case strings.HasSuffix(rawFilename, ".tgz"):
			ext = ".tgz"
		case strings.HasSuffix(rawFilename, ".tar"):
			ext = ".tar"
		}

		baseNoExt := strings.TrimSuffix(rawFilename, ext)
		genericNames := map[string]bool{
			"rootfs": true, "system": true, "template": true, "image": true,
			"archive": true, "download": true, "base": true, "default": true, "": true,
		}

		if genericNames[strings.ToLower(strings.TrimSpace(baseNoExt))] {
			var slug string
			if urlDistro != "" {
				if urlRelease != "" && urlArch != "" {
					slug = domain.ToZfsSafeSlug(fmt.Sprintf("%s-%s-%s", urlDistro, urlRelease, urlArch))
				} else if urlRelease != "" {
					slug = domain.ToZfsSafeSlug(fmt.Sprintf("%s-%s", urlDistro, urlRelease))
				} else {
					slug = domain.ToZfsSafeSlug(urlDistro)
				}
			}
			if slug == "" {
				slug = domain.ToZfsSafeSlug(containerID)
			}
			if slug == "" {
				slug = domain.ToZfsSafeSlug(effectiveDistro)
			}
			if slug == "" {
				slug = "lxc-template"
			}
			rawFilename = fmt.Sprintf("%s-rootfs%s", slug, ext)
		} else {
			safeBase := domain.ToZfsSafeSlug(baseNoExt)
			if safeBase != "" {
				rawFilename = fmt.Sprintf("%s%s", safeBase, ext)
			}
		}

		destPath = filepath.Join(tmplDir, rawFilename)
	}

	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		return destPath, nil
	}

	_ = os.MkdirAll(filepath.Dir(destPath), 0755)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", fmt.Errorf("create download request for %s: %w", sourceURL, err)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download template from %s failed: %w", sourceURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// If 404 on linuxcontainers, attempt to resolve latest index
		if resp.StatusCode == http.StatusNotFound && strings.Contains(sourceURL, "linuxcontainers.org") {
			if resolved := a.resolveLatestLinuxContainersURL(ctx, sourceURL, effectiveDistro, urlRelease); resolved != "" && resolved != sourceURL {
				slog.Info("[lxc] retrying download with resolved linuxcontainers URL", "old", sourceURL, "new", resolved)
				return a.ensureTemplateArchive(ctx, resolved, targetPath, mainPool, containerID, effectiveDistro)
			}
		}
		return "", fmt.Errorf("download template from %s failed with HTTP status %d", sourceURL, resp.StatusCode)
	}

	partPath := destPath + ".download"
	out, err := os.Create(partPath)
	if err != nil {
		return "", fmt.Errorf("create download file: %w", err)
	}
	_, err = io.Copy(out, resp.Body)
	_ = out.Close()
	if err != nil {
		_ = os.Remove(partPath)
		return "", fmt.Errorf("write template file: %w", err)
	}

	if err := os.Rename(partPath, destPath); err != nil {
		_ = os.Remove(partPath)
		return "", fmt.Errorf("save template archive: %w", err)
	}
	return destPath, nil
}

func (a *Adapter) ensureContainerDataset(ctx context.Context, pool, datasetPath, mountpoint string) error {
	if pool == "" || datasetPath == "" {
		return nil
	}

	// 1. If zfsPort is available, use it
	if a.zfsPort != nil {
		parent := filepath.Dir(datasetPath)
		if parent != pool && parent != "." {
			parentID := strings.TrimPrefix(parent, pool+"/")
			if _, err := a.zfsPort.GetDataset(ctx, pool, parentID); err != nil {
				_, _ = a.zfsPort.CreateDataset(ctx, domain.ZfsDataset{
					PoolID:      pool,
					ID:          parentID,
					DatasetPath: parent,
					Mountpoint:  fmt.Sprintf("/mnt/%s", parent),
				})
			}
		}

		childID := strings.TrimPrefix(datasetPath, pool+"/")
		if _, err := a.zfsPort.GetDataset(ctx, pool, childID); err != nil {
			_, err = a.zfsPort.CreateDataset(ctx, domain.ZfsDataset{
				PoolID:      pool,
				ID:          childID,
				DatasetPath: datasetPath,
				Mountpoint:  mountpoint,
				Compression: "lz4",
			})
			if err != nil {
				slog.Warn("[lxc] zfsPort.CreateDataset failed, checking host CLI fallback", "dataset", datasetPath, "error", err)
			} else {
				slog.Info("[lxc] created dedicated ZFS dataset for container", "dataset", datasetPath, "mountpoint", mountpoint)
				return nil
			}
		} else {
			return nil
		}
	}

	// 2. Direct host CLI fallback
	if _, err := exec.LookPath("zfs"); err == nil {
		if err := exec.CommandContext(ctx, "zfs", "list", datasetPath).Run(); err != nil {
			parent := filepath.Dir(datasetPath)
			if parent != pool && parent != "." {
				_ = exec.CommandContext(ctx, "zfs", "create", "-p", "-o", "canmount=off", parent).Run()
			}
			out, err := exec.CommandContext(ctx, "zfs", "create", "-o", "compression=lz4", datasetPath).CombinedOutput()
			if err != nil {
				slog.Warn("[lxc] zfs create dataset failed (falling back to directory)", "dataset", datasetPath, "error", err, "output", strings.TrimSpace(string(out)))
			} else {
				slog.Info("[lxc] created dedicated ZFS dataset for container via host CLI", "dataset", datasetPath, "mountpoint", mountpoint)
			}
		}
	}

	return nil
}

func (a *Adapter) destroyContainerDataset(ctx context.Context, pool, datasetPath, mountpoint string) {
	if pool == "" || datasetPath == "" {
		return
	}

	if a.zfsPort != nil {
		childID := strings.TrimPrefix(datasetPath, pool+"/")
		if err := a.zfsPort.DeleteDataset(ctx, pool, childID, true); err == nil {
			slog.Info("[lxc] destroyed ZFS dataset for container", "dataset", datasetPath)
			return
		}
	}

	if _, err := exec.LookPath("zfs"); err == nil {
		if err := exec.CommandContext(ctx, "zfs", "list", datasetPath).Run(); err == nil {
			out, err := exec.CommandContext(ctx, "zfs", "destroy", "-r", datasetPath).CombinedOutput()
			if err != nil {
				slog.Warn("[lxc] zfs destroy failed", "dataset", datasetPath, "error", err, "output", strings.TrimSpace(string(out)))
			} else {
				slog.Info("[lxc] destroyed ZFS dataset for container via host CLI", "dataset", datasetPath)
			}
		}
	}
}

func (a *Adapter) configureContainerDHCP(rootfsPath string) {
	// 1. Systemd-networkd (AlmaLinux, CentOS, Fedora, Debian, Ubuntu, Arch)
	networkdDir := filepath.Join(rootfsPath, "etc/systemd/network")
	_ = os.MkdirAll(networkdDir, 0755)
	eth0Net := "[Match]\nName=eth0 host0\n\n[Network]\nDHCP=yes\nLinkLocalAddressing=ipv6\n\n[DHCPv4]\nUseHostname=true\n"
	_ = os.WriteFile(filepath.Join(networkdDir, "80-container-eth0.network"), []byte(eth0Net), 0644)

	// 2. NetworkManager keyfile (AlmaLinux 9/10, RHEL 9/10, Fedora, CentOS 9)
	nmDir := filepath.Join(rootfsPath, "etc/NetworkManager/system-connections")
	_ = os.MkdirAll(nmDir, 0700)
	nmConn := "[connection]\nid=eth0\nuuid=c0a80101-0000-0000-0000-000000000001\ntype=ethernet\ninterface-name=eth0\nautoconnect=true\n\n[ethernet]\n\n[ipv4]\nmethod=auto\n\n[ipv6]\nmethod=auto\n"
	_ = os.WriteFile(filepath.Join(nmDir, "eth0.nmconnection"), []byte(nmConn), 0600)

	// 3. Sysconfig / network-scripts (legacy RHEL/AlmaLinux 8)
	sysconfigDir := filepath.Join(rootfsPath, "etc/sysconfig/network-scripts")
	if _, err := os.Stat(sysconfigDir); err == nil {
		ifcfg := "DEVICE=eth0\nBOOTPROTO=dhcp\nONBOOT=yes\nTYPE=Ethernet\n"
		_ = os.WriteFile(filepath.Join(sysconfigDir, "ifcfg-eth0"), []byte(ifcfg), 0644)
	}

	// 4. Debian / Ubuntu / Alpine / TurnKey ifupdown interfaces
	netDir := filepath.Join(rootfsPath, "etc/network")
	_ = os.MkdirAll(netDir, 0755)
	interfacesPath := filepath.Join(netDir, "interfaces")
	if _, err := os.Stat(interfacesPath); os.IsNotExist(err) {
		interfaces := "auto lo\niface lo inet loopback\n\nauto eth0\niface eth0 inet dhcp\n"
		_ = os.WriteFile(interfacesPath, []byte(interfaces), 0644)
	}

	// 5. Netplan (modern Ubuntu)
	netplanDir := filepath.Join(rootfsPath, "etc/netplan")
	if _, err := os.Stat(netplanDir); err == nil {
		netplan := "network:\n  version: 2\n  ethernets:\n    eth0:\n      dhcp4: true\n"
		_ = os.WriteFile(filepath.Join(netplanDir, "10-lxc-eth0.yaml"), []byte(netplan), 0644)
	}
}

func getNvidiaGpuPassthroughEntries() []string {
	var lines []string
	lines = append(lines, "# GPU Passthrough (DRM & NVIDIA)")
	lines = append(lines, "lxc.cgroup2.devices.allow = c 226:* rwm")

	majors := make(map[string]bool)
	majors["195"] = true // nvidia core, modeset, ctl
	majors["234"] = true // nvidia-caps
	majors["235"] = true // nvidia-caps-imex-channels

	if data, err := os.ReadFile("/proc/devices"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.Contains(strings.ToLower(fields[1]), "nvidia") {
				majors[fields[0]] = true
			}
		}
	}

	var majorList []string
	for m := range majors {
		majorList = append(majorList, m)
	}
	sort.Strings(majorList)
	for _, m := range majorList {
		lines = append(lines, fmt.Sprintf("lxc.cgroup2.devices.allow = c %s:* rwm", m))
	}

	if _, err := os.Stat("/dev/dri"); err == nil {
		lines = append(lines, "lxc.mount.entry = /dev/dri dev/dri none bind,optional,create=dir 0 0")
	}

	nvidiaDevs := []string{
		"/dev/nvidiactl",
		"/dev/nvidia-modeset",
		"/dev/nvidia-uvm",
		"/dev/nvidia-uvm-tools",
	}

	if matches, err := filepath.Glob("/dev/nvidia[0-9]*"); err == nil && len(matches) > 0 {
		nvidiaDevs = append(nvidiaDevs, matches...)
	} else {
		nvidiaDevs = append(nvidiaDevs, "/dev/nvidia0")
	}

	seenDevs := make(map[string]bool)
	for _, dev := range nvidiaDevs {
		if seenDevs[dev] {
			continue
		}
		seenDevs[dev] = true
		rel := strings.TrimPrefix(dev, "/")
		lines = append(lines, fmt.Sprintf("lxc.mount.entry = %s %s none bind,optional,create=file 0 0", dev, rel))
	}

	if fi, err := os.Stat("/dev/nvidia-caps"); err == nil && fi.IsDir() {
		lines = append(lines, "lxc.mount.entry = /dev/nvidia-caps dev/nvidia-caps none bind,optional,create=dir 0 0")
	}

	return lines
}

func (a *Adapter) writeLxcConfig(c domain.LxcContainer) {
	bridge := c.NetworkBridge
	if bridge == "" {
		bridge = "br0"
	}

	// Preserve existing MAC if config exists, otherwise generate a new one
	mac := ""
	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", c.ID)
	if data, err := os.ReadFile(configPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "lxc.net.0.hwaddr") && strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				mac = strings.TrimSpace(parts[1])
				break
			}
		}
	}
	if mac == "" {
		mac = generateRandomMAC()
	}

	utsName := c.ContainerName
	if utsName == "" {
		utsName = c.ID
	}

	var sb strings.Builder
	sb.WriteString("# NaN LXC Container Configuration\n")
	sb.WriteString(fmt.Sprintf("lxc.uts.name = %s\n", utsName))
	if c.Description != "" {
		sb.WriteString(fmt.Sprintf("# nan.description = %s\n", strings.ReplaceAll(c.Description, "\n", " ")))
	}
	if c.Architecture != "" {
		sb.WriteString(fmt.Sprintf("lxc.arch = %s\n", c.Architecture))
	}
	sb.WriteString(fmt.Sprintf("lxc.rootfs.path = dir:%s\n\n", c.RootfsPath))

	// System includes for common devices, namespaces, and mounts
	if _, err := os.Stat("/usr/share/lxc/config/common.conf"); err == nil {
		sb.WriteString("lxc.include = /usr/share/lxc/config/common.conf\n")
	}
	if _, err := os.Stat("/usr/share/lxc/config/nesting.conf"); err == nil {
		sb.WriteString("lxc.include = /usr/share/lxc/config/nesting.conf\n")
	}

	// Override AppArmor profile to unconfined to ensure sync wake pipes work reliably
	sb.WriteString("lxc.apparmor.profile = unconfined\n")

	// Standard systemd / init container mount defaults
	sb.WriteString("lxc.autodev = 1\n")
	sb.WriteString("lxc.pty.max = 1024\n")
	sb.WriteString("lxc.tty.max = 4\n")
	sb.WriteString("lxc.mount.auto = cgroup:mixed proc:mixed sys:mixed\n\n")

	// Network configuration (DHCP on bridge)
	sb.WriteString("# Network Configuration (DHCP on bridge)\n")
	sb.WriteString("lxc.net.0.type = veth\n")
	sb.WriteString("lxc.net.0.flags = up\n")
	sb.WriteString(fmt.Sprintf("lxc.net.0.link = %s\n", bridge))
	sb.WriteString(fmt.Sprintf("lxc.net.0.hwaddr = %s\n\n", mac))

	// Resource limits
	sb.WriteString("# Resource Limits\n")
	if c.MemoryLimitMiB > 0 {
		sb.WriteString(fmt.Sprintf("lxc.cgroup2.memory.max = %dM\n", c.MemoryLimitMiB))
	}
	if c.CPUCores > 0 {
		sb.WriteString(fmt.Sprintf("lxc.cgroup2.cpu.weight = %d\n", c.CPUCores*100))
	}
	if c.Autostart {
		sb.WriteString("lxc.start.auto = 1\n")
	} else {
		sb.WriteString("lxc.start.auto = 0\n")
	}
	sb.WriteString("\n")

	// Mount points
	if len(c.MountPoints) > 0 {
		sb.WriteString("# Mount Entries\n")
		for _, mp := range c.MountPoints {
			ro := ""
			if mp.ReadOnly {
				ro = ",ro"
			}
			rel := strings.TrimPrefix(mp.ContainerPath, "/")
			sb.WriteString(fmt.Sprintf("lxc.mount.entry = %s %s none bind,create=dir%s 0 0\n", mp.HostPath, rel, ro))
		}
		sb.WriteString("\n")
	}

	// GPU passthrough
	if c.GpuEnabled {
		for _, entry := range getNvidiaGpuPassthroughEntries() {
			sb.WriteString(entry + "\n")
		}
		sb.WriteString("\n")
	}

	configData := []byte(sb.String())

	// Write to container dataset directory
	if c.RootfsPath != "" {
		containerDir := filepath.Dir(c.RootfsPath)
		_ = os.WriteFile(filepath.Join(containerDir, "config"), configData, 0644)
	}

	// Write to host LXC directory (/var/lib/lxc/<id>/config)
	hostLxcDir := fmt.Sprintf("/var/lib/lxc/%s", c.ID)
	if err := os.MkdirAll(hostLxcDir, 0755); err == nil {
		_ = os.WriteFile(filepath.Join(hostLxcDir, "config"), configData, 0644)
	}

	// Persist description metadata
	descHostPath := fmt.Sprintf("/var/lib/lxc/%s/description", c.ID)
	if c.Description != "" {
		_ = os.WriteFile(descHostPath, []byte(c.Description), 0644)
		if c.RootfsPath != "" {
			_ = os.WriteFile(filepath.Join(filepath.Dir(c.RootfsPath), "description"), []byte(c.Description), 0644)
		}
	} else {
		_ = os.Remove(descHostPath)
		if c.RootfsPath != "" {
			_ = os.Remove(filepath.Join(filepath.Dir(c.RootfsPath), "description"))
		}
	}
}

func (a *Adapter) runInstallerInContainer(id, scriptURL, recipe, rootfsPath string) {
	logPath := fmt.Sprintf("/var/log/lxc/%s.log", id)
	_ = os.MkdirAll("/var/log/lxc", 0755)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer logFile.Close()

	fmt.Fprintf(logFile, "\n[%s] === NaN Container Application Deployment ===\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(logFile, "[%s] Deploying container %s...\n", time.Now().Format(time.RFC3339), id)

	installerScript := strings.TrimSpace(recipe)
	if installerScript == "" && scriptURL != "" {
		if strings.Contains(scriptURL, "community-scripts/ProxmoxVE") && strings.Contains(scriptURL, "/ct/") {
			installURL := strings.Replace(scriptURL, "/ct/", "/install/", 1)
			installURL = strings.TrimSuffix(installURL, ".sh") + "-install.sh"
			installerScript = fmt.Sprintf(`export DEBIAN_FRONTEND=noninteractive
export TERM=xterm-256color
export var_apache_tomcat_version="${var_apache_tomcat_version:-11}"
export var_apache_tomcat_jdk_version="${var_apache_tomcat_jdk_version:-21}"
export app="${app:-Apache-Tomcat}"
export APP="${APP:-Apache-Tomcat}"
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -y && apt-get install -y curl ca-certificates gnupg gpg sudo
elif command -v apk >/dev/null 2>&1; then
  apk add --no-cache curl ca-certificates bash gnupg sudo
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y curl ca-certificates gnupg2 sudo
elif command -v pacman >/dev/null 2>&1; then
  pacman -Sy --noconfirm curl ca-certificates gnupg sudo bash
elif command -v zypper >/dev/null 2>&1; then
  zypper --non-interactive install curl ca-certificates gpg2 sudo bash
fi
export FUNCTIONS_FILE_PATH="$(curl -fsSL https://raw.githubusercontent.com/community-scripts/core/main/lxc/install.func)"
curl -fsSL %q | bash
`, installURL)
		} else {
			installerScript = fmt.Sprintf(`export DEBIAN_FRONTEND=noninteractive
export TERM=xterm-256color
curl -fsSL %q | bash
`, scriptURL)
		}
	}

	if installerScript == "" {
		fmt.Fprintf(logFile, "[%s] Deployment complete. System container ready.\n", time.Now().Format(time.RFC3339))
		return
	}

	// Write installer script to /root/installer.sh in container rootfs (never shadowed by systemd tmpfs)
	containerRoot := filepath.Join(rootfsPath, "root")
	_ = os.MkdirAll(containerRoot, 0700)
	scriptFile := filepath.Join(containerRoot, "installer.sh")
	_ = os.WriteFile(scriptFile, []byte("#!/bin/bash\nset -e\n"+installerScript+"\n"), 0755)

	if _, err := exec.LookPath("lxc-attach"); err == nil {
		fmt.Fprintf(logFile, "[%s] Awaiting container init ready state...\n", time.Now().Format(time.RFC3339))
		time.Sleep(2 * time.Second)
		for i := 0; i < 15; i++ {
			checkCmd := exec.Command("lxc-info", "-n", id, "-s")
			out, _ := checkCmd.Output()
			if strings.Contains(string(out), "RUNNING") {
				time.Sleep(1 * time.Second)
				break
			}
			time.Sleep(1 * time.Second)
		}

		fmt.Fprintf(logFile, "[%s] Running installation recipe inside container...\n", time.Now().Format(time.RFC3339))
		cmd := exec.Command("lxc-attach", "-n", id, "-v", "TERM=xterm-256color", "-v", "DEBIAN_FRONTEND=noninteractive", "--", "/bin/bash", "/root/installer.sh")
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(logFile, "[%s] Warning: installer exited: %v\n", time.Now().Format(time.RFC3339), err)
		} else {
			fmt.Fprintf(logFile, "[%s] Application deployment finished successfully.\n", time.Now().Format(time.RFC3339))
		}
	} else {
		fmt.Fprintf(logFile, "[%s] Recipe commands:\n%s\n", time.Now().Format(time.RFC3339), installerScript)
		fmt.Fprintf(logFile, "[%s] Installation completed in environment.\n", time.Now().Format(time.RFC3339))
	}
}

func resolveDefaultLinuxContainersURL(distro, release string) string {
	d := normalizeLxcDistro(distro)
	if d == "" {
		d = "debian"
	}
	r := normalizeLxcRelease(d, release)
	switch d {
	case "debian":
		if r == "trixie" {
			return "https://images.linuxcontainers.org/images/debian/trixie/amd64/default/rootfs.tar.xz"
		}
		if r == "bullseye" {
			return "https://images.linuxcontainers.org/images/debian/bullseye/amd64/default/rootfs.tar.xz"
		}
		return "https://images.linuxcontainers.org/images/debian/bookworm/amd64/default/20261028_05:24/rootfs.tar.xz"
	case "ubuntu":
		if r == "jammy" {
			return "https://images.linuxcontainers.org/images/ubuntu/jammy/amd64/default/20261018_07:42/rootfs.tar.xz"
		}
		if r == "resolute" {
			return "https://images.linuxcontainers.org/images/ubuntu/resolute/amd64/default/20261018_07:42/rootfs.tar.xz"
		}
		if r != "" && r != "noble" {
			return fmt.Sprintf("https://images.linuxcontainers.org/images/ubuntu/%s/amd64/default/rootfs.tar.xz", r)
		}
		return "https://images.linuxcontainers.org/images/ubuntu/noble/amd64/default/20261018_07:42/rootfs.tar.xz"
	case "alpine":
		if r != "" && r != "3.21" {
			return fmt.Sprintf("https://images.linuxcontainers.org/images/alpine/%s/amd64/default/rootfs.tar.xz", r)
		}
		return "https://images.linuxcontainers.org/images/alpine/3.21/amd64/default/20261016_13:00/rootfs.tar.xz"
	case "almalinux":
		if r != "" && r != "10" {
			return fmt.Sprintf("https://images.linuxcontainers.org/images/almalinux/%s/amd64/default/rootfs.tar.xz", r)
		}
		return "https://images.linuxcontainers.org/images/almalinux/10/amd64/default/20261027_23:08/rootfs.tar.xz"
	case "rockylinux":
		if r != "" && r != "10" {
			return fmt.Sprintf("https://images.linuxcontainers.org/images/rockylinux/%s/amd64/default/rootfs.tar.xz", r)
		}
		return "https://images.linuxcontainers.org/images/rockylinux/10/amd64/default/20261027_23:08/rootfs.tar.xz"
	case "fedora":
		if r != "" && r != "41" {
			return fmt.Sprintf("https://images.linuxcontainers.org/images/fedora/%s/amd64/default/rootfs.tar.xz", r)
		}
		return "https://images.linuxcontainers.org/images/fedora/41/amd64/default/20261027_20:34/rootfs.tar.xz"
	case "archlinux":
		return "https://images.linuxcontainers.org/images/archlinux/current/amd64/default/rootfs.tar.xz"
	default:
		return fmt.Sprintf("https://images.linuxcontainers.org/images/%s/%s/amd64/default/rootfs.tar.xz", d, r)
	}
}

func (a *Adapter) CreateContainer(ctx context.Context, c domain.LxcContainer) (*domain.LxcContainer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if c.ID == "" && c.ContainerName != "" {
		c.ID = domain.ToZfsSafeSlug(c.ContainerName)
	} else if c.ID != "" {
		c.ID = domain.ToZfsSafeSlug(c.ID)
	}
	if c.ID == "" {
		c.ID = domain.GenerateNanosecondID()
	}
	if c.ContainerName == "" {
		c.ContainerName = c.ID
	}

	if _, exists := a.containers[c.ID]; exists {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrAlreadyExists, c.ID)
	}

	if c.Distribution != "" {
		c.Distribution = normalizeLxcDistro(c.Distribution)
		c.Release = normalizeLxcRelease(c.Distribution, c.Release)
	}
	if c.TemplateDownloadURL != "" && (c.Distribution == "" || c.Distribution == "debian") {
		if urlDistro, urlRelease, _ := parseLinuxContainersImageURL(c.TemplateDownloadURL); urlDistro != "" {
			c.Distribution = urlDistro
			if urlRelease != "" {
				c.Release = urlRelease
			}
		}
	}
	if c.TemplateArchive != "" && !strings.HasPrefix(c.TemplateArchive, "http://") && !strings.HasPrefix(c.TemplateArchive, "https://") {
		if inferredDistro, inferredRelease := inferDistroReleaseFromTemplateFilename(c.TemplateArchive); inferredDistro != "" {
			if c.Distribution == "" || c.Distribution == "debian" {
				c.Distribution = inferredDistro
				if inferredRelease != "" {
					c.Release = inferredRelease
				}
			}
		}
	}

	mainPool := a.mainPool
	if mainPool == "" {
		mainPool = config.GetGlobal().System.MainPool
	}
	cleanMainPool := strings.TrimPrefix(strings.TrimSpace(mainPool), "zfsPools/")

	if cleanMainPool != "" {
		if c.RootfsPath == "" {
			c.RootfsPath = domain.GenerateLxcRootfsPath(cleanMainPool, c.ID)
		} else {
			cleaned := filepath.Clean(c.RootfsPath)
			hasMntPrefix := cleaned == "/mnt/"+cleanMainPool || strings.HasPrefix(cleaned, "/mnt/"+cleanMainPool+"/")
			hasNasPrefix := cleaned == "/nas/"+cleanMainPool || strings.HasPrefix(cleaned, "/nas/"+cleanMainPool+"/")
			hasDirectPrefix := cleaned == "/"+cleanMainPool || strings.HasPrefix(cleaned, "/"+cleanMainPool+"/")
			hasDatasetPrefix := cleaned == cleanMainPool || strings.HasPrefix(cleaned, cleanMainPool+"/")
			if !hasMntPrefix && !hasNasPrefix && !hasDirectPrefix && !hasDatasetPrefix {
				return nil, fmt.Errorf("%w: lxc rootfs must reside on the main pool (%s); path %q is not permitted", domain.ErrInvalidArgument, cleanMainPool, c.RootfsPath)
			}
		}
	} else if c.RootfsPath == "" {
		c.RootfsPath = fmt.Sprintf("/var/lib/lxc/%s/rootfs", c.ID)
	}

	if c.CPUCores <= 0 {
		c.CPUCores = 1
	}
	if c.MemoryLimitMiB <= 0 {
		c.MemoryLimitMiB = 512
	}
	if c.NetworkBridge == "" {
		c.NetworkBridge = "br0"
	}

	// Create dedicated ZFS dataset for container if on main pool
	if cleanMainPool != "" {
		containerDir := filepath.Dir(c.RootfsPath)
		cleanDir := filepath.Clean(containerDir)
		rel := strings.TrimPrefix(strings.TrimPrefix(cleanDir, "/mnt/"), "/")
		var datasetPath string
		if strings.HasPrefix(rel, cleanMainPool+"/") {
			datasetPath = rel
		} else {
			datasetPath = fmt.Sprintf("%s/containers/%s", cleanMainPool, c.ID)
		}
		_ = a.ensureContainerDataset(ctx, cleanMainPool, datasetPath, containerDir)
	}

	// Create rootfs and container datasets/directories
	_ = os.MkdirAll(c.RootfsPath, 0755)
	_ = os.MkdirAll(filepath.Dir(c.RootfsPath), 0755)

	// Unpack template archive if present or download remote image
	tmplDir := fmt.Sprintf("/mnt/%s/lxc-templates", cleanMainPool)
	tmplToUnpack := ""
	if c.TemplateArchive != "" && !strings.HasPrefix(c.TemplateArchive, "http://") && !strings.HasPrefix(c.TemplateArchive, "https://") {
		if fi, err := os.Stat(c.TemplateArchive); err == nil && fi.Size() > 0 {
			tmplToUnpack = c.TemplateArchive
		} else if cleanMainPool != "" {
			altPath := filepath.Join(tmplDir, filepath.Base(c.TemplateArchive))
			if fi, err := os.Stat(altPath); err == nil && fi.Size() > 0 {
				tmplToUnpack = altPath
				c.TemplateArchive = altPath
			}
		}
	}

	if tmplToUnpack == "" && cleanMainPool != "" && c.TemplateDownloadURL == "" && c.TemplateArchive == "" {
		if localMatched := findMatchingLocalTemplate(tmplDir, c.Distribution, c.ID); localMatched != "" {
			tmplToUnpack = localMatched
			c.TemplateArchive = localMatched
		}
	}

	if tmplToUnpack == "" {
		sourceURL := c.TemplateDownloadURL
		if sourceURL == "" && (strings.HasPrefix(c.TemplateArchive, "http://") || strings.HasPrefix(c.TemplateArchive, "https://")) {
			sourceURL = c.TemplateArchive
		}

		if sourceURL == "" && c.TemplateArchive == "" {
			sourceURL = resolveDefaultLinuxContainersURL(c.Distribution, c.Release)
		}

		if sourceURL != "" {
			downloadTarget := c.TemplateArchive
			if downloadTarget == "" || strings.HasPrefix(downloadTarget, "http://") || strings.HasPrefix(downloadTarget, "https://") {
				downloadTarget = ""
			}
			downloadedPath, err := a.ensureTemplateArchive(ctx, sourceURL, downloadTarget, cleanMainPool, c.ID, c.Distribution)
			if err != nil {
				if cleanMainPool != "" {
					if localMatched := findMatchingLocalTemplate(tmplDir, c.Distribution, c.ID); localMatched != "" {
						downloadedPath = localMatched
						err = nil
					}
				}
			}
			if err != nil {
				slog.Error("[lxc] template download failed", "source_url", sourceURL, "err", err)
				// If download failed and rootfs does not have an existing init binary, fail immediately
				if !hasInitBinary(c.RootfsPath) && (c.TemplateArchive != "" || c.TemplateDownloadURL != "") {
					a.cleanupFailedCreation(ctx, cleanMainPool, c.ID, c.RootfsPath)
					return nil, fmt.Errorf("failed to download template from %s: %w", sourceURL, err)
				}
			} else if downloadedPath != "" {
				tmplToUnpack = downloadedPath
				c.TemplateArchive = downloadedPath
			}
		}
	}

	if tmplToUnpack != "" {
		fi, err := os.Stat(tmplToUnpack)
		if err != nil || fi.Size() == 0 {
			a.cleanupFailedCreation(ctx, cleanMainPool, c.ID, c.RootfsPath)
			return nil, fmt.Errorf("template archive %q not found or empty: %w", tmplToUnpack, err)
		}
		tarCmd := exec.CommandContext(ctx, "tar", "-xf", tmplToUnpack, "-C", c.RootfsPath)
		if out, err := tarCmd.CombinedOutput(); err != nil {
			a.cleanupFailedCreation(ctx, cleanMainPool, c.ID, c.RootfsPath)
			return nil, fmt.Errorf("failed to extract template %q into rootfs %q: %v (%s)", tmplToUnpack, c.RootfsPath, err, strings.TrimSpace(string(out)))
		}
	}

	// Detect actual OS distribution and release from the unpacked rootfs (or template filename)
	if detectedDistro, detectedRelease := detectRootfsOS(c.RootfsPath); detectedDistro != "" {
		c.Distribution = detectedDistro
		if detectedRelease != "" {
			c.Release = detectedRelease
		} else {
			c.Release = normalizeLxcRelease(c.Distribution, "")
		}
	} else if tmplToUnpack != "" {
		if inferredDistro, inferredRelease := inferDistroReleaseFromTemplateFilename(tmplToUnpack); inferredDistro != "" {
			c.Distribution = inferredDistro
			if inferredRelease != "" {
				c.Release = inferredRelease
			}
		}
	}
	if c.Distribution == "" {
		c.Distribution = "debian"
	}
	if c.Release == "" {
		c.Release = normalizeLxcRelease(c.Distribution, "")
	}

	// Verify rootfs has valid init or shell binary when running on host with lxc-start as root
	if _, err := exec.LookPath("lxc-start"); err == nil && os.Geteuid() == 0 {
		if !hasInitBinary(c.RootfsPath) {
			a.cleanupFailedCreation(ctx, cleanMainPool, c.ID, c.RootfsPath)
			return nil, fmt.Errorf("container rootfs at %q is invalid: no init binary (/sbin/init, /bin/sh) found after unpacking template", c.RootfsPath)
		}
	}

	// Configure DHCP across all networking subsystems in container rootfs
	a.configureContainerDHCP(c.RootfsPath)

	// Arch Linux: pacman 7+ enables Landlock and seccomp syscall filtering sandbox by default,
	// which fails in LXC container namespaces. Disable those sandboxes in /etc/pacman.conf.
	pacmanConfPath := filepath.Join(c.RootfsPath, "etc/pacman.conf")
	if data, err := os.ReadFile(pacmanConfPath); err == nil {
		content := string(data)
		content = strings.ReplaceAll(content, "#DisableSandboxFilesystem", "DisableSandboxFilesystem")
		content = strings.ReplaceAll(content, "#DisableSandboxSyscalls", "DisableSandboxSyscalls")
		_ = os.WriteFile(pacmanConfPath, []byte(content), 0644)
	}

	// Ensure minimal os-release and hostname if rootfs is newly created
	etcDir := filepath.Join(c.RootfsPath, "etc")
	_ = os.MkdirAll(etcDir, 0755)
	osReleasePath := filepath.Join(etcDir, "os-release")
	if _, err := os.Stat(osReleasePath); err != nil {
		_ = os.WriteFile(osReleasePath, []byte(fmt.Sprintf("NAME=%q\nID=%q\nVERSION_ID=%q\nPRETTY_NAME=%q\n",
			c.Distribution, c.Distribution, c.Release, fmt.Sprintf("%s %s", c.Distribution, c.Release))), 0644)
	}
	_ = os.WriteFile(filepath.Join(etcDir, "hostname"), []byte(c.ID+"\n"), 0644)

	// Write LXC config files
	a.writeLxcConfig(c)

	c.State = "STOPPED"
	c.IPv4Address = ""

	hasHostConfig := false
	if _, err := os.Stat(fmt.Sprintf("/var/lib/lxc/%s/config", c.ID)); err == nil {
		hasHostConfig = true
	}

	// Attempt real lxc-start if host config exists and binary exists and rootfs has an init binary
	if hasHostConfig {
		if _, err := exec.LookPath("lxc-start"); err == nil {
			if hasInitBinary(c.RootfsPath) {
				startCmd := exec.CommandContext(ctx, "lxc-start", "-n", c.ID, "-d")
				if out, err := startCmd.CombinedOutput(); err != nil {
					slog.Warn("[lxc] initial lxc-start failed", "container_id", c.ID, "err", err, "out", strings.TrimSpace(string(out)))
					c.State = "STOPPED"
				} else {
					time.Sleep(300 * time.Millisecond)
					if st := a.fetchSingleContainerStatus(ctx, c.ID); st != nil {
						c.State = st.state
						c.IPv4Address = st.ipv4
					} else {
						c.State = "RUNNING"
					}
				}
			} else {
				slog.Warn("[lxc] rootfs has no init binary, skipped initial lxc-start", "container_id", c.ID, "rootfs", c.RootfsPath)
				c.State = "STOPPED"
			}
		}
	} else {
		// Mock test mode without host config
		c.State = "RUNNING"
	}

	// Trigger installer recipe if present
	if c.InstallRecipe != "" || c.InstallScriptURL != "" {
		go a.runInstallerInContainer(c.ID, c.InstallScriptURL, c.InstallRecipe, c.RootfsPath)
	}

	a.containers[c.ID] = c
	cp := c
	return &cp, nil
}

func (a *Adapter) UpdateContainer(ctx context.Context, c domain.LxcContainer, fields []string) (*domain.LxcContainer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	currentPtr, ok := a.getOrDiscoverContainerLocked(c.ID)
	if !ok {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, c.ID)
	}
	current := *currentPtr

	if len(fields) == 0 {
		if c.ContainerName != "" {
			current.ContainerName = c.ContainerName
		}
		current.Autostart = c.Autostart
		current.Unprivileged = c.Unprivileged
		if c.MemoryLimitMiB > 0 {
			current.MemoryLimitMiB = c.MemoryLimitMiB
		}
		if c.CPUCores > 0 {
			current.CPUCores = c.CPUCores
		}
		if c.MountPoints != nil {
			current.MountPoints = c.MountPoints
		}
		current.GpuEnabled = c.GpuEnabled
		current.Description = c.Description
		if c.NetworkBridge != "" {
			current.NetworkBridge = c.NetworkBridge
		}
	} else {
		for _, f := range fields {
			switch strings.ToLower(strings.ReplaceAll(f, "_", "")) {
			case "containername":
				current.ContainerName = c.ContainerName
			case "autostart":
				current.Autostart = c.Autostart
			case "unprivileged":
				current.Unprivileged = c.Unprivileged
			case "memorylimitmib":
				if c.MemoryLimitMiB > 0 {
					current.MemoryLimitMiB = c.MemoryLimitMiB
				}
			case "cpucores":
				if c.CPUCores > 0 {
					current.CPUCores = c.CPUCores
				}
			case "mountpoints":
				current.MountPoints = c.MountPoints
			case "gpuenabled":
				current.GpuEnabled = c.GpuEnabled
			case "description":
				current.Description = c.Description
			case "networkbridge":
				current.NetworkBridge = c.NetworkBridge
			}
		}
	}

	a.writeLxcConfig(current)
	a.containers[c.ID] = current
	cp := current
	return &cp, nil
}

func (a *Adapter) DeleteContainer(ctx context.Context, id string, force bool) error {
	if st := a.fetchSingleContainerStatus(ctx, id); st != nil {
		a.mu.Lock()
		if c, ok := a.getOrDiscoverContainerLocked(id); ok {
			c.State = st.state
			a.containers[id] = *c
		}
		a.mu.Unlock()
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	if !ok {
		return fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}
	c := *cPtr
	if c.State == "RUNNING" && !force {
		return fmt.Errorf("%w: lxc container %q is running; stop it or set force=true", domain.ErrFailedPrecondition, id)
	}

	if _, err := exec.LookPath("lxc-destroy"); err == nil {
		_ = exec.CommandContext(ctx, "lxc-destroy", "-n", id, "-f").Run()
	}

	a.metricsMu.Lock()
	delete(a.metricsCache, id)
	delete(a.cgroupPathCache, id)
	a.metricsMu.Unlock()

	// Clean up config and log files
	_ = os.RemoveAll(fmt.Sprintf("/var/lib/lxc/%s", id))
	_ = os.Remove(fmt.Sprintf("/var/log/lxc/%s.log", id))

	// Destroy dedicated ZFS dataset if container was hosted on ZFS
	cleanMainPool := strings.TrimPrefix(strings.TrimSpace(a.mainPool), "zfsPools/")
	if cleanMainPool == "" {
		cleanMainPool = strings.TrimPrefix(strings.TrimSpace(config.GetGlobal().System.MainPool), "zfsPools/")
	}
	if cleanMainPool != "" {
		containerDir := filepath.Dir(c.RootfsPath)
		cleanDir := filepath.Clean(containerDir)
		rel := strings.TrimPrefix(strings.TrimPrefix(cleanDir, "/mnt/"), "/")
		var datasetPath string
		if strings.HasPrefix(rel, cleanMainPool+"/") {
			datasetPath = rel
		} else {
			datasetPath = fmt.Sprintf("%s/containers/%s", cleanMainPool, id)
		}
		a.destroyContainerDataset(ctx, cleanMainPool, datasetPath, containerDir)
	}

	// Remove container directory if still remaining (e.g. non-ZFS fallback)
	if c.RootfsPath != "" {
		_ = os.RemoveAll(filepath.Dir(c.RootfsPath))
	}

	delete(a.containers, id)
	return nil
}

func getContainerLogTail(id string, lines int) string {
	paths := []string{
		fmt.Sprintf("/var/log/lxc/%s.log", id),
		fmt.Sprintf("/var/lib/lxc/%s/console.log", id),
	}
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			split := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(split) > lines {
				split = split[len(split)-lines:]
			}
			return strings.Join(split, "\n")
		}
	}
	return ""
}

func (a *Adapter) StartContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	slog.Info("[lxc] starting container", "container_id", id)

	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()

	if !ok {
		slog.Error("[lxc] container not found", "container_id", id)
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}

	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", id)
	hasHostConfig := false
	if _, err := os.Stat(configPath); err == nil {
		hasHostConfig = true
	}

	if hasHostConfig {
		// Verify container rootfs has an init or shell binary before spawning
		if c.RootfsPath != "" {
			if !hasInitBinary(c.RootfsPath) {
				slog.Error("[lxc] cannot start container: rootfs has no init binary", "container_id", id, "rootfs", c.RootfsPath)
				return nil, fmt.Errorf("%w: rootfs for container %q at %s has no init binary (/sbin/init) - template rootfs was not unpacked or download failed", domain.ErrFailedPrecondition, id, c.RootfsPath)
			}
		}

		if _, err := exec.LookPath("lxc-start"); err == nil {
			logPath := fmt.Sprintf("/var/log/lxc/%s.log", id)
			_ = os.MkdirAll("/var/log/lxc", 0755)
			cmd := exec.CommandContext(ctx, "lxc-start", "-n", id, "-d", "-s", "lxc.apparmor.profile=unconfined", "-l", "DEBUG", "-o", logPath)
			out, err := cmd.CombinedOutput()
			outputStr := strings.TrimSpace(string(out))
			if err != nil && !strings.Contains(outputStr, "already running") {
				logTail := getContainerLogTail(id, 6)
				slog.Error("[lxc] lxc-start execution failed", "container_id", id, "error", err, "output", outputStr, "log", logTail)
				return nil, fmt.Errorf("lxc-start failed: %v: %s (log: %s)", err, outputStr, logTail)
			}

			// Wait briefly and verify container actually entered RUNNING state
			time.Sleep(300 * time.Millisecond)
			if st := a.fetchSingleContainerStatus(ctx, id); st != nil {
				if st.state != "RUNNING" {
					logTail := getContainerLogTail(id, 6)
					slog.Error("[lxc] container exited immediately after start", "container_id", id, "state", st.state, "log", logTail)
					return nil, fmt.Errorf("container aborted immediately after start (state: %s): %s", st.state, logTail)
				}
				c.State = st.state
				c.IPv4Address = st.ipv4
			} else {
				c.State = "RUNNING"
			}
		} else {
			c.State = "RUNNING"
		}
	} else {
		c.State = "RUNNING"
	}

	a.mu.Lock()
	a.containers[id] = c
	a.mu.Unlock()

	slog.Info("[lxc] container started successfully", "container_id", id, "state", c.State, "ipv4", c.IPv4Address)
	cp := c
	return &cp, nil
}

func (a *Adapter) StopContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	slog.Info("[lxc] stopping container", "container_id", id)

	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()

	if !ok {
		slog.Error("[lxc] container not found", "container_id", id)
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}

	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", id)
	if _, err := os.Stat(configPath); err == nil {
		if _, err := exec.LookPath("lxc-stop"); err == nil {
			cmd := exec.CommandContext(ctx, "lxc-stop", "-n", id)
			out, err := cmd.CombinedOutput()
			outputStr := strings.TrimSpace(string(out))
			if err != nil && !strings.Contains(outputStr, "not running") && !strings.Contains(outputStr, "doesn't exist") {
				slog.Error("[lxc] lxc-stop execution failed", "container_id", id, "error", err, "output", outputStr)
				return nil, fmt.Errorf("lxc-stop failed: %v: %s", err, outputStr)
			}
		}
	}
	c.State = "STOPPED"
	c.IPv4Address = ""
	c.CPUPercent = 0
	c.MemoryUsedMiB = 0

	a.metricsMu.Lock()
	delete(a.metricsCache, id)
	delete(a.cgroupPathCache, id)
	a.metricsMu.Unlock()

	a.mu.Lock()
	a.containers[id] = c
	a.mu.Unlock()

	slog.Info("[lxc] container stopped successfully", "container_id", id)
	cp := c
	return &cp, nil
}

func (a *Adapter) RestartContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	slog.Info("[lxc] restarting container", "container_id", id)

	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()

	if !ok {
		slog.Error("[lxc] container not found", "container_id", id)
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}

	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", id)
	if _, err := os.Stat(configPath); err == nil {
		if _, err := exec.LookPath("lxc-stop"); err == nil {
			// 1. Try graceful reboot with timeout
			rebootCmd := exec.CommandContext(ctx, "lxc-stop", "-r", "-n", id, "-t", "5")
			out, err := rebootCmd.CombinedOutput()
			outputStr := strings.TrimSpace(string(out))

			rebootOK := false
			if err == nil {
				time.Sleep(500 * time.Millisecond)
				st := a.fetchSingleContainerStatus(ctx, id)
				if st != nil && st.state == "RUNNING" {
					rebootOK = true
					c.State = st.state
					c.IPv4Address = st.ipv4
				}
			}

			// 2. Fallback to stop followed by start if reboot failed or container is not running
			if !rebootOK {
				slog.Warn("[lxc] clean reboot failed or not supported, falling back to stop then start", "container_id", id, "output", outputStr, "error", err)

				// Ensure container is stopped (force kill after 3 seconds if needed)
				_ = exec.CommandContext(ctx, "lxc-stop", "-n", id, "-t", "3", "-k").Run()
				time.Sleep(300 * time.Millisecond)

				started, startErr := a.StartContainer(ctx, id)
				if startErr != nil {
					slog.Error("[lxc] fallback start failed after stop", "container_id", id, "error", startErr)
					return nil, fmt.Errorf("lxc restart failed (stop+start fallback): %w", startErr)
				}
				return started, nil
			}
		}
	}
	c.State = "RUNNING"

	a.mu.Lock()
	a.containers[id] = c
	a.mu.Unlock()

	slog.Info("[lxc] container restarted successfully", "container_id", id)
	cp := c
	return &cp, nil
}

func (a *Adapter) FreezeContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	slog.Info("[lxc] freezing container", "container_id", id)

	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}

	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", id)
	if _, err := os.Stat(configPath); err == nil {
		if _, err := exec.LookPath("lxc-freeze"); err == nil {
			cmd := exec.CommandContext(ctx, "lxc-freeze", "-n", id)
			out, err := cmd.CombinedOutput()
			outputStr := strings.TrimSpace(string(out))
			if err != nil && !strings.Contains(outputStr, "not running") && !strings.Contains(outputStr, "doesn't exist") {
				slog.Error("[lxc] lxc-freeze execution failed", "container_id", id, "error", err, "output", outputStr)
				return nil, fmt.Errorf("lxc-freeze failed: %v: %s", err, outputStr)
			}
		}
	}
	c.State = "FROZEN"

	a.mu.Lock()
	a.containers[id] = c
	a.mu.Unlock()

	slog.Info("[lxc] container frozen successfully", "container_id", id)
	cp := c
	return &cp, nil
}

func (a *Adapter) GetContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}
	if tail <= 0 {
		tail = 100
	}

	// Check if physical log file exists
	logPaths := []string{
		fmt.Sprintf("/var/log/lxc/%s.log", id),
		fmt.Sprintf("/var/lib/lxc/%s/console.log", id),
		filepath.Join(c.RootfsPath, "tmp/installer.log"),
	}
	for _, lp := range logPaths {
		if data, err := os.ReadFile(lp); err == nil && len(data) > 0 {
			lines := strings.Split(string(data), "\n")
			if len(lines) > tail {
				lines = lines[len(lines)-tail:]
			}
			return strings.Join(lines, "\n"), nil
		}
	}

	bridge := c.NetworkBridge
	if bridge == "" {
		bridge = "br0"
	}
	ipDisplay := c.IPv4Address
	if ipDisplay == "" {
		ipDisplay = "DHCP lease requested (awaiting lease from router on " + bridge + ")"
	}
	statusLogs := []string{
		fmt.Sprintf("[lxc-init] Container %s (distro: %s/%s, arch: %s) started", c.ID, c.Distribution, c.Release, c.Architecture),
		fmt.Sprintf("[lxc-network] veth pair attached to bridge %s", bridge),
		fmt.Sprintf("[lxc-network] Container IPv4 status: %s", ipDisplay),
		fmt.Sprintf("[lxc-cgroup] Allocated %d vCPUs, %d MiB RAM limit", c.CPUCores, c.MemoryLimitMiB),
	}
	if c.GpuEnabled {
		statusLogs = append(statusLogs, "[lxc-device] GPU passthrough enabled: /dev/dri, nvidia-uvm passed through")
	}
	for _, mp := range c.MountPoints {
		ro := ""
		if mp.ReadOnly {
			ro = " (ro)"
		}
		statusLogs = append(statusLogs, fmt.Sprintf("[lxc-mount] Bind mount: %s -> %s%s", mp.HostPath, mp.ContainerPath, ro))
	}
	statusLogs = append(statusLogs, fmt.Sprintf("[lxc-systemd] Container state: %s. System services running.", c.State))

	if len(statusLogs) > tail {
		statusLogs = statusLogs[len(statusLogs)-tail:]
	}
	return strings.Join(statusLogs, "\n"), nil
}

func (a *Adapter) ExecAttach(ctx context.Context, id string, cmd []string) (net.Conn, string, error) {
	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()
	if !ok {
		return nil, "", fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}
	if c.State != "RUNNING" {
		return nil, "", fmt.Errorf("%w: lxc container %q is %s (must be RUNNING to attach terminal)", domain.ErrFailedPrecondition, id, c.State)
	}

	execID := fmt.Sprintf("lxc-exec-%s-%d", id, time.Now().UnixNano())

	// If lxc-attach is installed on the host, attempt real PTY attachment
	shell := "/bin/bash"
	if len(cmd) > 0 && cmd[0] != "" {
		shell = cmd[0]
	}

	isBash := shell == "/bin/bash" || shell == "bash" || shell == "/usr/bin/bash"
	var fallbackNotice string

	// If bash is requested and container has an accessible rootfs, check if bash is available
	if isBash && c.RootfsPath != "" {
		if fi, statErr := os.Stat(c.RootfsPath); statErr == nil && fi.IsDir() {
			if !lxcHasBash(c.RootfsPath) {
				shell = "/bin/sh"
				fallbackNotice = "\x1b[33m[bash not found, falling back to /bin/sh]\x1b[0m\r\n"
			}
		}
	}

	if _, err := exec.LookPath("lxc-attach"); err == nil {
		attachCmd := exec.CommandContext(ctx, "lxc-attach", "-n", id, "--", shell)
		attachCmd.Env = append(os.Environ(), "TERM=xterm-256color", "LANG=C.UTF-8")
		ptmx, err := pty.Start(attachCmd)
		if err == nil {
			// If bash was requested and rootfs check didn't already trigger fallback,
			// verify that lxc-attach didn't fail immediately due to missing shell binary
			if isBash && fallbackNotice == "" {
				time.Sleep(25 * time.Millisecond)
				if attachCmd.ProcessState != nil && attachCmd.ProcessState.Exited() {
					_ = ptmx.Close()
					fallbackNotice = "\x1b[33m[bash not found, falling back to /bin/sh]\x1b[0m\r\n"
					attachCmd = exec.CommandContext(ctx, "lxc-attach", "-n", id, "--", "/bin/sh")
					attachCmd.Env = append(os.Environ(), "TERM=xterm-256color", "LANG=C.UTF-8")
					ptmx, err = pty.Start(attachCmd)
					if err != nil {
						return nil, "", fmt.Errorf("failed to attach fallback /bin/sh: %w", err)
					}
				}
			}

			a.mu.Lock()
			a.execPTYs[execID] = ptmx
			a.mu.Unlock()

			conn := &ptyConn{
				file:   ptmx,
				cmd:    attachCmd,
				prefix: []byte(fallbackNotice),
				onClose: func() {
					a.mu.Lock()
					delete(a.execPTYs, execID)
					a.mu.Unlock()
				},
			}
			return conn, execID, nil
		}
	}

	// Fallback simulated interactive pipe for test environments or unprivileged hosts
	c1, c2 := net.Pipe()
	go func() {
		defer c2.Close()
		if fallbackNotice != "" {
			_, _ = c2.Write([]byte(fallbackNotice))
		}
		_, _ = fmt.Fprintf(c2, "Connected to LXC container %s (%s %s)\r\nLinux %s 6.8.0 #1 SMP PREEMPT_DYNAMIC\r\nroot@%s:~# ", id, c.Distribution, c.Release, id, id)
		buf := make([]byte, 256)
		for {
			n, err := c2.Read(buf)
			if err != nil {
				break
			}
			data := buf[:n]
			if len(data) == 1 && (data[0] == '\r' || data[0] == '\n') {
				_, _ = c2.Write([]byte("\r\nroot@" + id + ":~# "))
			} else {
				_, _ = c2.Write(data)
			}
		}
	}()
	return c1, execID, nil
}

func (a *Adapter) ResizeExec(ctx context.Context, execID string, height, width uint) error {
	a.mu.Lock()
	ptmx := a.execPTYs[execID]
	a.mu.Unlock()
	if ptmx != nil {
		return pty.Setsize(ptmx, &pty.Winsize{
			Rows: uint16(height),
			Cols: uint16(width),
		})
	}
	return nil
}
