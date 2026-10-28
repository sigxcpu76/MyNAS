package share

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
)

type fileItem struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
	URL     string
	Icon    string
	Type    string
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
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func getFileIconAndType(name string, isDir bool) (string, string) {
	if isDir {
		return `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#38bdf8" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>`, "Directory"
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".bmp", ".ico":
		return `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#a78bfa" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>`, "Image"
	case ".mp4", ".mkv", ".avi", ".mov", ".webm", ".flv":
		return `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#f43f5e" stroke-width="2"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2"/></svg>`, "Video"
	case ".mp3", ".flac", ".wav", ".ogg", ".m4a", ".aac":
		return `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#fb923c" stroke-width="2"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>`, "Audio"
	case ".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar", ".zst", ".iso":
		return `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#eab308" stroke-width="2"><polyline points="21 8 21 21 3 21 3 8"/><rect x="1" y="3" width="22" height="5"/><line x1="10" y1="12" x2="14" y2="12"/></svg>`, "Archive"
	case ".pdf":
		return `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#ef4444" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>`, "PDF Document"
	case ".txt", ".md", ".log", ".csv", ".json", ".yaml", ".yml", ".xml", ".html", ".css", ".js", ".ts", ".go", ".py", ".sh", ".c", ".cpp":
		return `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#34d399" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>`, "Document"
	default:
		return `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#94a3b8" stroke-width="2"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><polyline points="13 2 13 9 20 9"/></svg>`, "File"
	}
}

func serveAutoIndex(w http.ResponseWriter, r *http.Request, share domain.WebdavShare, relPath, diskPath string) {
	entries, err := os.ReadDir(diskPath)
	if err != nil {
		http.Error(w, "Could not read directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var items []fileItem
	var totalDirs, totalFiles int
	var totalSize int64

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		isDir := entry.IsDir()
		var size int64
		if isDir {
			totalDirs++
		} else {
			size = info.Size()
			totalFiles++
			totalSize += size
		}

		encodedName := url.PathEscape(entry.Name())
		href := encodedName
		if isDir {
			href += "/"
		}

		icon, typeLabel := getFileIconAndType(entry.Name(), isDir)

		items = append(items, fileItem{
			Name:    entry.Name(),
			IsDir:   isDir,
			Size:    size,
			ModTime: info.ModTime(),
			URL:     href,
			Icon:    icon,
			Type:    typeLabel,
		})
	}

	// Sort directories first (case-insensitive A-Z), then files (case-insensitive A-Z)
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	// Build breadcrumb segments
	cleanRel := strings.Trim(filepath.Clean("/"+relPath), "/")
	var breadcrumbs []struct{ Title, URL string }
	breadcrumbs = append(breadcrumbs, struct{ Title, URL string }{"Shares", "/dav/"})
	breadcrumbs = append(breadcrumbs, struct{ Title, URL string }{share.ID, "/dav/" + share.ID + "/"})

	if cleanRel != "" && cleanRel != "." {
		parts := strings.Split(cleanRel, "/")
		acc := "/dav/" + share.ID + "/"
		for _, p := range parts {
			acc += url.PathEscape(p) + "/"
			breadcrumbs = append(breadcrumbs, struct{ Title, URL string }{p, acc})
		}
	}

	isRoot := (cleanRel == "" || cleanRel == ".")
	parentURL := "../"

	host := r.Host
	if host == "" {
		host = "localhost"
	}
	mountURL := fmt.Sprintf("http://%s/dav/%s/", host, share.ID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	// Stream HTML Auto-Index
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Index of /%s/%s - NAS WebDAV</title>
<style>
  :root {
    --bg: #0f172a;
    --card-bg: #1e293b;
    --border: #334155;
    --text-primary: #f8fafc;
    --text-secondary: #94a3b8;
    --accent: #38bdf8;
    --accent-hover: #0284c7;
    --badge-bg: rgba(56, 189, 248, 0.15);
    --hover-row: rgba(255, 255, 255, 0.04);
  }
  @media (prefers-color-scheme: light) {
    :root {
      --bg: #f8fafc;
      --card-bg: #ffffff;
      --border: #e2e8f0;
      --text-primary: #0f172a;
      --text-secondary: #64748b;
      --accent: #0284c7;
      --accent-hover: #0369a1;
      --badge-bg: rgba(2, 132, 199, 0.1);
      --hover-row: rgba(0, 0, 0, 0.02);
    }
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    background-color: var(--bg);
    color: var(--text-primary);
    padding: 24px 16px;
    font-size: 14px;
    line-height: 1.5;
  }
  .container {
    max-width: 1100px;
    margin: 0 auto;
  }
  .card {
    background: var(--card-bg);
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1), 0 2px 4px -2px rgba(0,0,0,0.1);
    overflow: hidden;
  }
  .header {
    padding: 18px 24px;
    border-bottom: 1px solid var(--border);
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 12px;
  }
  .title-group {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .logo {
    display: flex;
    align-items: center;
    color: var(--accent);
  }
  .breadcrumbs {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
    font-weight: 500;
  }
  .breadcrumbs a {
    color: var(--accent);
    text-decoration: none;
  }
  .breadcrumbs a:hover {
    text-decoration: underline;
  }
  .breadcrumbs span.sep {
    color: var(--text-secondary);
  }
  .breadcrumbs span.current {
    color: var(--text-primary);
    font-weight: 600;
  }
  .badges {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 12px;
    font-weight: 600;
    padding: 3px 8px;
    border-radius: 6px;
    border: 1px solid var(--border);
  }
  .badge-ro {
    background: rgba(234, 179, 8, 0.15);
    color: #eab308;
    border-color: rgba(234, 179, 8, 0.3);
  }
  .badge-rw {
    background: rgba(34, 197, 94, 0.15);
    color: #22c55e;
    border-color: rgba(34, 197, 94, 0.3);
  }
  .badge-info {
    background: var(--badge-bg);
    color: var(--accent);
    border-color: rgba(56, 189, 248, 0.3);
  }
  .toolbar {
    padding: 12px 24px;
    background: rgba(0, 0, 0, 0.1);
    border-bottom: 1px solid var(--border);
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 12px;
  }
  .filter-input {
    background: var(--bg);
    border: 1px solid var(--border);
    color: var(--text-primary);
    border-radius: 6px;
    padding: 6px 12px;
    font-size: 13px;
    width: 240px;
    outline: none;
  }
  .filter-input:focus {
    border-color: var(--accent);
  }
  .mount-hint {
    font-size: 12px;
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    gap: 6px;
    font-family: monospace;
  }
  .copy-btn {
    background: var(--bg);
    border: 1px solid var(--border);
    color: var(--text-primary);
    border-radius: 4px;
    padding: 2px 8px;
    font-size: 11px;
    cursor: pointer;
  }
  .copy-btn:hover {
    border-color: var(--accent);
  }
  table {
    width: 100%%;
    border-collapse: collapse;
    text-align: left;
  }
  th {
    padding: 10px 24px;
    font-size: 12px;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    border-bottom: 1px solid var(--border);
  }
  td {
    padding: 12px 24px;
    border-bottom: 1px solid var(--border);
    font-size: 13.5px;
  }
  tr.item-row:hover td {
    background-color: var(--hover-row);
  }
  .name-cell {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .name-link {
    color: var(--text-primary);
    text-decoration: none;
    font-weight: 500;
  }
  .name-link:hover {
    color: var(--accent);
    text-decoration: underline;
  }
  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 12.5px;
  }
  .actions-cell {
    text-align: right;
  }
  .dl-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    background: transparent;
    border: 1px solid var(--border);
    color: var(--text-secondary);
    padding: 4px 10px;
    border-radius: 4px;
    font-size: 12px;
    text-decoration: none;
  }
  .dl-btn:hover {
    color: var(--accent);
    border-color: var(--accent);
  }
  .footer {
    padding: 14px 24px;
    font-size: 12px;
    color: var(--text-secondary);
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
  }
</style>
</head>
<body>
<div class="container">
  <div class="card">
    <div class="header">
      <div class="title-group">
        <div class="logo">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z"/><line x1="2" y1="10" x2="22" y2="10"/></svg>
        </div>
        <nav class="breadcrumbs">
`, html.EscapeString(share.ID), html.EscapeString(cleanRel))

	for idx, b := range breadcrumbs {
		if idx == len(breadcrumbs)-1 {
			fmt.Fprintf(w, `<span class="current">%s</span>`, html.EscapeString(b.Title))
		} else {
			fmt.Fprintf(w, `<a href="%s">%s</a><span class="sep">/</span>`, html.EscapeString(b.URL), html.EscapeString(b.Title))
		}
	}

	modeClass := "badge-rw"
	modeLabel := "Read-Write"
	if share.ReadOnly {
		modeClass = "badge-ro"
		modeLabel = "Read-Only"
	}

	fmt.Fprintf(w, `
        </nav>
      </div>
      <div class="badges">
        <span class="badge %s">%s</span>
        <span class="badge badge-info">%d items</span>
      </div>
    </div>

    <div class="toolbar">
      <input type="text" id="filter" class="filter-input" placeholder="Filter files..." oninput="filterItems(this.value)">
      <div class="mount-hint">
        <span>Mount URL: <strong>%s</strong></span>
        <button class="copy-btn" onclick="navigator.clipboard.writeText('%s'); this.innerText='Copied!'; setTimeout(()=>this.innerText='Copy', 1500)">Copy</button>
      </div>
    </div>

    <table>
      <thead>
        <tr>
          <th>Name</th>
          <th>Type</th>
          <th>Size</th>
          <th>Last Modified</th>
          <th style="text-align: right">Action</th>
        </tr>
      </thead>
      <tbody id="file-list">
`, modeClass, modeLabel, len(items), html.EscapeString(mountURL), html.EscapeString(mountURL))

	if !isRoot {
		fmt.Fprintf(w, `
        <tr class="item-row">
          <td>
            <div class="name-cell">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#94a3b8" stroke-width="2"><polyline points="15 18 9 12 15 6"/></svg>
              <a href="%s" class="name-link">.. [Parent Directory]</a>
            </div>
          </td>
          <td style="color: var(--text-secondary)">Parent Directory</td>
          <td class="mono" style="color: var(--text-secondary)">—</td>
          <td class="mono" style="color: var(--text-secondary)">—</td>
          <td></td>
        </tr>
`, parentURL)
	}

	if len(items) == 0 {
		fmt.Fprint(w, `
        <tr>
          <td colspan="5" style="text-align: center; color: var(--text-secondary); padding: 36px">
            Empty folder. No files or directories found.
          </td>
        </tr>
`)
	} else {
		for _, it := range items {
			sizeStr := "—"
			if !it.IsDir {
				sizeStr = formatBytes(it.Size)
			}
			actionLabel := "Open"
			if !it.IsDir {
				actionLabel = "Download"
			}
			fmt.Fprintf(w, `
        <tr class="item-row" data-name="%s">
          <td>
            <div class="name-cell">
              %s
              <a href="%s" class="name-link">%s</a>
            </div>
          </td>
          <td style="color: var(--text-secondary)">%s</td>
          <td class="mono">%s</td>
          <td class="mono" style="color: var(--text-secondary)">%s</td>
          <td class="actions-cell">
            <a href="%s" class="dl-btn"%s>%s</a>
          </td>
        </tr>
`,
				html.EscapeString(strings.ToLower(it.Name)),
				it.Icon,
				html.EscapeString(it.URL),
				html.EscapeString(it.Name),
				html.EscapeString(it.Type),
				sizeStr,
				it.ModTime.Format("2006-01-02 15:04:05"),
				html.EscapeString(it.URL),
				func() string {
					if !it.IsDir {
						return " download"
					}
					return ""
				}(),
				actionLabel,
			)
		}
	}

	fmt.Fprintf(w, `
      </tbody>
    </table>

    <div class="footer">
      <div>Directory: <span class="mono">%s</span></div>
      <div>%d folders, %d files (%s total) • NAS WebDAV Service</div>
    </div>
  </div>
</div>
<script>
function filterItems(q) {
  var rows = document.querySelectorAll('#file-list tr.item-row');
  var query = q.trim().toLowerCase();
  for (var i = 0; i < rows.length; i++) {
    var name = rows[i].getAttribute('data-name');
    if (!name) continue;
    rows[i].style.display = name.indexOf(query) !== -1 ? '' : 'none';
  }
}
</script>
</body>
</html>
`, html.EscapeString(diskPath), totalDirs, totalFiles, formatBytes(totalSize))
}

func serveSharesIndex(w http.ResponseWriter, r *http.Request, shares []domain.WebdavShare) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	host := r.Host
	if host == "" {
		host = "localhost"
	}

	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>NAS WebDAV Service - Shares Index</title>
<style>
  :root {
    --bg: #0f172a;
    --card-bg: #1e293b;
    --border: #334155;
    --text-primary: #f8fafc;
    --text-secondary: #94a3b8;
    --accent: #38bdf8;
    --hover-row: rgba(255, 255, 255, 0.04);
  }
  @media (prefers-color-scheme: light) {
    :root {
      --bg: #f8fafc;
      --card-bg: #ffffff;
      --border: #e2e8f0;
      --text-primary: #0f172a;
      --text-secondary: #64748b;
      --accent: #0284c7;
      --hover-row: rgba(0, 0, 0, 0.02);
    }
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    background-color: var(--bg);
    color: var(--text-primary);
    padding: 32px 16px;
    font-size: 14px;
    line-height: 1.5;
  }
  .container { max-width: 1000px; margin: 0 auto; }
  .card {
    background: var(--card-bg);
    border: 1px solid var(--border);
    border-radius: 10px;
    overflow: hidden;
  }
  .header {
    padding: 20px 24px;
    border-bottom: 1px solid var(--border);
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  h1 { font-size: 18px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
  table { width: 100%%; border-collapse: collapse; text-align: left; }
  th {
    padding: 10px 24px;
    font-size: 12px;
    color: var(--text-secondary);
    text-transform: uppercase;
    border-bottom: 1px solid var(--border);
  }
  td { padding: 14px 24px; border-bottom: 1px solid var(--border); }
  tr:hover td { background-color: var(--hover-row); }
  .name-cell a { color: var(--accent); text-decoration: none; font-weight: 600; font-size: 14px; }
  .name-cell a:hover { text-decoration: underline; }
  .badge {
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 600;
  }
  .badge-ro { background: rgba(234, 179, 8, 0.15); color: #eab308; }
  .badge-rw { background: rgba(34, 197, 94, 0.15); color: #22c55e; }
  .btn {
    display: inline-block;
    padding: 4px 10px;
    border-radius: 4px;
    background: var(--accent);
    color: #fff;
    text-decoration: none;
    font-size: 12px;
    font-weight: 500;
  }
</style>
</head>
<body>
<div class="container">
  <div class="card">
    <div class="header">
      <h1>
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z"/><line x1="2" y1="10" x2="22" y2="10"/></svg>
        Available WebDAV Shares
      </h1>
      <span style="font-size: 13px; color: var(--text-secondary)">%d configured</span>
    </div>
    <table>
      <thead>
        <tr>
          <th>Share ID</th>
          <th>Filesystem Path & Description</th>
          <th>Access</th>
          <th>Allowed</th>
          <th style="text-align: right">Action</th>
        </tr>
      </thead>
      <tbody>
`, len(shares))

	if len(shares) == 0 {
		fmt.Fprint(w, `
        <tr>
          <td colspan="5" style="text-align: center; color: var(--text-secondary); padding: 36px">
            No active WebDAV shares configured.
          </td>
        </tr>
`)
	} else {
		for _, s := range shares {
			modeClass := "badge-rw"
			modeLabel := "RW"
			if s.ReadOnly {
				modeClass = "badge-ro"
				modeLabel = "RO"
			}
			allowed := "all authenticated"
			if len(s.AllowedUsers) > 0 {
				allowed = strings.Join(s.AllowedUsers, ", ")
			}
			shareHref := fmt.Sprintf("/dav/%s/", s.ID)

			fmt.Fprintf(w, `
        <tr>
          <td class="name-cell">
            <a href="%s">%s</a>
          </td>
          <td>
            <div style="font-family: monospace; font-size: 12px">%s</div>
            <div style="font-size: 11px; color: var(--text-secondary)">%s</div>
          </td>
          <td><span class="badge %s">%s</span></td>
          <td style="font-size: 12px; color: var(--text-secondary)">%s</td>
          <td style="text-align: right">
            <a href="%s" class="btn">Browse</a>
          </td>
        </tr>
`,
				shareHref,
				html.EscapeString(s.ID),
				html.EscapeString(s.Path),
				html.EscapeString(s.Description),
				modeClass,
				modeLabel,
				html.EscapeString(allowed),
				shareHref,
			)
		}
	}

	fmt.Fprint(w, `
      </tbody>
    </table>
  </div>
</div>
</body>
</html>
`)
}
