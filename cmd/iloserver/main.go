package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	portFlag = flag.Int("port", 8080, "Port to listen on")
	bindFlag = flag.String("bind", "0.0.0.0", "Address to bind on")
	dirFlag  = flag.String("dir", "", "Directory containing ISOs (default: ~/Downloads or current dir)")
)

// responseLogger wraps http.ResponseWriter to capture status code and bytes written
type responseLogger struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (l *responseLogger) WriteHeader(status int) {
	l.status = status
	l.ResponseWriter.WriteHeader(status)
}

func (l *responseLogger) Write(b []byte) (int, error) {
	if l.status == 0 {
		l.status = http.StatusOK
	}
	n, err := l.ResponseWriter.Write(b)
	l.bytes += int64(n)
	return n, err
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

type IsoItem struct {
	Name    string
	Size    string
	ModTime string
	Url     string
}

const htmlTemplate = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>iLO / BMC Virtual Media Server</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, monospace; background: #0f172a; color: #f1f5f9; padding: 30px; margin: 0; }
    h1 { color: #38bdf8; margin-bottom: 6px; font-size: 24px; }
    p.sub { color: #94a3b8; font-size: 14px; margin-top: 0; margin-bottom: 24px; }
    table { width: 100%; border-collapse: collapse; background: #1e293b; border-radius: 8px; overflow: hidden; }
    th { background: #334155; text-align: left; padding: 12px 16px; font-size: 13px; color: #cbd5e1; }
    td { padding: 12px 16px; border-bottom: 1px solid #334155; font-size: 13px; }
    tr:last-child td { border-bottom: none; }
    tr:hover td { background: #283548; }
    .btn { background: #0284c7; color: white; border: none; padding: 6px 12px; border-radius: 4px; cursor: pointer; text-decoration: none; font-size: 12px; }
    .btn:hover { background: #0369a1; }
    .mono { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    .url-box { background: #0f172a; padding: 4px 8px; border-radius: 4px; color: #38bdf8; font-size: 12px; border: 1px solid #475569; user-select: all; }
  </style>
</head>
<body>
  <h1>iLO 4 / 5 Scripted Media HTTP Server</h1>
  <p class="sub">Directory: <span class="mono">{{ .Dir }}</span> &bull; Local IP: <span class="mono">{{ .HostIP }}:{{ .Port }}</span></p>
  
  <table>
    <thead>
      <tr>
        <th>File Name</th>
        <th>Size</th>
        <th>Last Modified</th>
        <th>iLO Scripted Media URL (Copy into iLO)</th>
        <th>Action</th>
      </tr>
    </thead>
    <tbody>
      {{ range .Files }}
      <tr>
        <td style="font-weight: 600;">{{ .Name }}</td>
        <td>{{ .Size }}</td>
        <td style="color: #94a3b8;">{{ .ModTime }}</td>
        <td><span class="url-box mono">{{ .Url }}</span></td>
        <td><a class="btn" href="{{ .Url }}" download>Download</a></td>
      </tr>
      {{ else }}
      <tr>
        <td colspan="5" style="text-align: center; color: #94a3b8; padding: 30px;">No .iso files found in directory.</td>
      </tr>
      {{ end }}
    </tbody>
  </table>
</body>
</html>`

func getPrimaryIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		return localAddr.IP.String()
	}
	return "127.0.0.1"
}

func main() {
	flag.Parse()

	dir := *dirFlag
	if dir == "" {
		home, _ := os.UserHomeDir()
		downloads := filepath.Join(home, "Downloads")
		if stat, err := os.Stat(downloads); err == nil && stat.IsDir() {
			dir = downloads
		} else {
			dir = "."
		}
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		log.Fatalf("Invalid directory: %v", err)
	}

	hostIP := getPrimaryIP()
	port := *portFlag
	addr := fmt.Sprintf("%s:%d", *bindFlag, port)

	log.Printf("=================================================================")
	log.Printf("   iLO 4 / 5 Scripted Virtual Media HTTP Server (Go RFC 7233)    ")
	log.Printf("=================================================================")
	log.Printf("  Serving Directory: %s", absDir)
	log.Printf("  Listening Address: http://%s:%d/", hostIP, port)
	log.Printf("=================================================================")

	tmpl := template.Must(template.New("index").Parse(htmlTemplate))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		logger := &responseLogger{ResponseWriter: w}

		// Set essential headers for HP iLO 4 Virtual Media
		logger.Header().Set("Accept-Ranges", "bytes")
		logger.Header().Set("Access-Control-Allow-Origin", "*")
		logger.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		logger.Header().Set("Access-Control-Allow-Headers", "Range, Content-Type, Accept")
		logger.Header().Set("Connection", "keep-alive")

		if r.Method == http.MethodOptions {
			logger.WriteHeader(http.StatusOK)
			return
		}

		cleanPath := filepath.Clean(r.URL.Path)

		// Root dashboard
		if cleanPath == "/" || cleanPath == "." {
			entries, err := os.ReadDir(absDir)
			if err != nil {
				http.Error(logger, "Failed to read directory", http.StatusInternalServerError)
				return
			}

			var files []IsoItem
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if strings.HasSuffix(strings.ToLower(name), ".iso") || strings.HasSuffix(strings.ToLower(name), ".img") {
					info, err := entry.Info()
					if err != nil {
						continue
					}
					files = append(files, IsoItem{
						Name:    name,
						Size:    formatBytes(info.Size()),
						ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
						Url:     fmt.Sprintf("http://%s:%d/%s", hostIP, port, name),
					})
				}
			}

			logger.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = tmpl.Execute(logger, map[string]interface{}{
				"Dir":    absDir,
				"HostIP": hostIP,
				"Port":   port,
				"Files":  files,
			})
			return
		}

		// File request
		relPath := strings.TrimPrefix(cleanPath, "/")
		fullPath := filepath.Join(absDir, relPath)

		// Security: prevent directory traversal outside absDir
		if !strings.HasPrefix(fullPath, absDir) {
			http.Error(logger, "Forbidden", http.StatusForbidden)
			return
		}

		file, err := os.Open(fullPath)
		if err != nil {
			http.NotFound(logger, r)
			log.Printf("[%s] %s %s -> 404 Not Found (Remote: %s)", time.Now().Format("15:04:05"), r.Method, cleanPath, r.RemoteAddr)
			return
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil || stat.IsDir() {
			http.NotFound(logger, r)
			return
		}

		// Explicit Content-Type required by HP iLO 4
		lower := strings.ToLower(stat.Name())
		if strings.HasSuffix(lower, ".iso") || strings.HasSuffix(lower, ".img") {
			logger.Header().Set("Content-Type", "application/octet-stream")
		}

		rangeHeader := r.Header.Get("Range")

		// Serve using Go standard library ServeContent (implements RFC 7233 range requests)
		http.ServeContent(logger, r, stat.Name(), stat.ModTime(), file)

		duration := time.Since(start)
		rangeInfo := ""
		if rangeHeader != "" {
			rangeInfo = fmt.Sprintf(" [Range: %s]", rangeHeader)
		}

		log.Printf("[%s] %s %s -> %d (%s in %v)%s | Client: %s | UA: %s",
			time.Now().Format("15:04:05"),
			r.Method,
			cleanPath,
			logger.status,
			formatBytes(logger.bytes),
			duration.Round(time.Millisecond),
			rangeInfo,
			r.RemoteAddr,
			r.UserAgent(),
		)
	})

	server := &http.Server{
		Addr:        addr,
		Handler:     handler,
		ReadTimeout: 0, // No timeout for slow firmware reads during BIOS POST
		WriteTimeout: 0,
		IdleTimeout: 10 * time.Minute,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
