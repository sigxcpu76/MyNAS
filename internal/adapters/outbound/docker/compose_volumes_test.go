package docker

import (
	"strings"
	"testing"
)

func TestNormalizeComposeVolumes_UserElasticsearchCase(t *testing.T) {
	// Exact input reported by user from TrueNAS questions wizard
	rawCompose := `services:
  elastic-search:
    container_name: elastic-search
    image: docker.elastic.co/elasticsearch/elasticsearch:9.5.5
    restart: unless-stopped
    ports:
      - "30003:8080"
    environment:
      - HEAP_SIZE=512m
      - NODE_NAME=elastic_search_node
    volumes:
      - ix_volume:/type
      - false:/acl_enable
      - data:/dataset_name
      - false:/acl_enable
      - /mnt/fast/apps/elastic-search/path:/path
    mem_limit: 512m
    cpus: 1
x-nas-management-port: 30003
x-portals:
  - name: Port Number
    scheme: http
    port: 30003
    path: /
`

	normalized := NormalizeComposeVolumes("elastic-search", "fast", rawCompose)

	// Ensure all invalid TrueNAS setting volume lines were purged
	if strings.Contains(normalized, "ix_volume") {
		t.Errorf("expected ix_volume to be purged or rewritten, got:\n%s", normalized)
	}
	if strings.Contains(normalized, "acl_enable") {
		t.Errorf("expected acl_enable setting to be purged, got:\n%s", normalized)
	}
	if strings.Contains(normalized, "dataset_name") {
		t.Errorf("expected dataset_name setting to be purged, got:\n%s", normalized)
	}
	if strings.Contains(normalized, ":/path") {
		t.Errorf("expected /path target to be rewritten, got:\n%s", normalized)
	}

	// Ensure the real container storage path is mounted
	if !strings.Contains(normalized, "/mnt/fast/apps/elastic-search/data:/usr/share/elasticsearch/data") {
		t.Errorf("expected /mnt/fast/apps/elastic-search/data:/usr/share/elasticsearch/data, got:\n%s", normalized)
	}

	// Ensure port 8080 was rewritten to Elasticsearch default port 9200
	if !strings.Contains(normalized, "30003:9200") {
		t.Errorf("expected 30003:9200 port mapping, got:\n%s", normalized)
	}

	// Ensure single-node and memory settings were injected
	if !strings.Contains(normalized, "discovery.type=single-node") {
		t.Errorf("expected discovery.type=single-node, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, "xpack.security.enabled=false") {
		t.Errorf("expected xpack.security.enabled=false, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, "ES_JAVA_OPTS=-Xms512m -Xmx512m") {
		t.Errorf("expected ES_JAVA_OPTS=-Xms512m -Xmx512m, got:\n%s", normalized)
	}
}

func TestNormalizeComposeVolumes_IxVolumeWithRealTarget(t *testing.T) {
	rawCompose := `services:
  jellyfin:
    image: jellyfin/jellyfin:latest
    volumes:
      - ix_volume:/config
      - ix_volume:/cache
`
	normalized := NormalizeComposeVolumes("jellyfin", "tank", rawCompose)

	if strings.Contains(normalized, "ix_volume:") {
		t.Errorf("expected ix_volume to be rewritten to host path, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, "/mnt/tank/apps/jellyfin/config:/config") {
		t.Errorf("expected /mnt/tank/apps/jellyfin/config:/config, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, "/mnt/tank/apps/jellyfin/cache:/cache") {
		t.Errorf("expected /mnt/tank/apps/jellyfin/cache:/cache, got:\n%s", normalized)
	}
}

func TestNormalizeComposeVolumes_AllSettingsDroppedInjectsDefaultData(t *testing.T) {
	rawCompose := `services:
  myapp:
    image: myapp:latest
    volumes:
      - ix_volume:/type
      - false:/acl_enable
`
	normalized := NormalizeComposeVolumes("myapp", "mypool", rawCompose)

	if strings.Contains(normalized, "type") || strings.Contains(normalized, "acl_enable") {
		t.Errorf("expected settings to be dropped, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, "/mnt/mypool/apps/myapp/data:/data") {
		t.Errorf("expected default data mount, got:\n%s", normalized)
	}
}

func TestNormalizeComposeVolumes_ESPHomeRelativeTargetAndFallbackRemoval(t *testing.T) {
	rawCompose := `x-nas-management-port: 36052
x-portals:
  - name: "Web UI"
    scheme: "http"
    port: 36052
    path: "/"
services:
  esphome:
    image: ghcr.io/esphome/esphome:2026.10.0
    container_name: esphome
    restart: unless-stopped
    ports:
      - "36052:80"
    environment:
      - PUID=568
      - PGID=568
      - TZ=Etc/UTC
    volumes:
      - /mnt/tank/apps/esphome:/data
      - /mnt/fast/apps/esphome/config:config
`
	normalized := NormalizeComposeVolumes("esphome", "fast", rawCompose)

	// Ensure :config was rewritten to :/config
	if strings.Contains(normalized, ":config\n") || strings.Contains(normalized, ":config\"") {
		t.Errorf("expected :config to be converted to :/config, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, "/mnt/fast/apps/esphome/config:/config") {
		t.Errorf("expected /mnt/fast/apps/esphome/config:/config, got:\n%s", normalized)
	}
	// Ensure the redundant fallback /mnt/fast/apps/esphome:/data was dropped
	if strings.Contains(normalized, ":/data") {
		t.Errorf("expected generic fallback :/data to be removed when specific /config exists, got:\n%s", normalized)
	}
}
