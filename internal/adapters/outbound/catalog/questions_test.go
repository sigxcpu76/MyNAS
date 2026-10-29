package catalog

import (
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

const sampleTrueNASQuestionsYAML = `
groups:
  - name: Actual Budget Configuration
    description: Configure Actual Budget
  - name: User and Group Configuration
    description: Configure User and Group for Actual Budget
  - name: Network Configuration
    description: Configure Network for Actual Budget
  - name: Storage Configuration
    description: Configure Storage for Actual Budget

questions:
  - variable: actual_budget
    schema:
      type: dict
      attrs:
        - variable: password
          label: Admin Password
          description: Password for Actual Budget
          schema:
            type: string
            default: "mysecret"
  - variable: network
    schema:
      type: dict
      attrs:
        - variable: web_port
          label: Web Port
          description: Listening web port
          schema:
            type: int
            default: 5006
  - variable: storage
    schema:
      type: dict
      attrs:
        - variable: data
          label: Data Storage
          description: Main data storage directory
          schema:
            type: hostpath
            mount_path: /data
            default: /mnt/tank/actual_budget/data
  - variable: run_as
    schema:
      type: dict
      attrs:
        - variable: user
          label: User ID
          schema:
            type: int
            default: 1000
        - variable: group
          label: Group ID
          schema:
            type: int
            default: 1000
`

func TestParseQuestionsYAML(t *testing.T) {
	groups, flat := ParseQuestionsYAML([]byte(sampleTrueNASQuestionsYAML), "actual-budget", "tank", "/mnt/tank/apps")

	if len(flat) == 0 {
		t.Fatalf("expected parsed questions, got none")
	}

	// Verify questions were categorized into groups
	if len(groups) == 0 {
		t.Fatalf("expected parsed question groups, got none")
	}

	var foundPort, foundStorage, foundPassword, foundUser bool
	for _, q := range flat {
		if strings.Contains(q.Variable, "web_port") {
			foundPort = true
			if q.Type != "int" || q.DefaultValue != "5006" {
				t.Errorf("unexpected port question: %+v", q)
			}
		}
		if strings.Contains(q.Variable, "storage.data") {
			foundStorage = true
			if q.Type != "hostpath" {
				t.Errorf("expected hostpath type, got %s", q.Type)
			}
			if !strings.HasPrefix(q.DefaultValue, "/mnt/") {
				t.Errorf("expected /mnt/ path, got %s", q.DefaultValue)
			}
			if q.MountPath != "/data" {
				t.Errorf("expected mount path /data, got %s", q.MountPath)
			}
		}
		if strings.Contains(q.Variable, "password") {
			foundPassword = true
			if q.DefaultValue != "mysecret" {
				t.Errorf("expected mysecret, got %s", q.DefaultValue)
			}
		}
		if strings.Contains(q.Variable, "run_as.user") {
			foundUser = true
			if q.DefaultValue != "1000" {
				t.Errorf("expected user id 1000, got %s", q.DefaultValue)
			}
		}
	}

	if !foundPort || !foundStorage || !foundPassword || !foundUser {
		t.Errorf("missing expected questions: port=%v storage=%v pass=%v user=%v", foundPort, foundStorage, foundPassword, foundUser)
	}
}

func TestGenerateComposeFromQuestions(t *testing.T) {
	app := domain.CatalogApp{
		AppID:          "actual-budget",
		ContainerImage: "ghcr.io/actualbudget/actual-server:latest",
		DefaultPort:    5006,
	}
	_, flat := ParseQuestionsYAML([]byte(sampleTrueNASQuestionsYAML), "actual-budget", "tank", "/mnt/tank/apps")
	app.Questions = flat

	answers := map[string]string{
		"network.web_port": "5050",
		"actual_budget.password": "custompass123",
	}

	compose := GenerateComposeFromQuestions(app, answers, "tank", "/mnt/tank/apps")

	if !strings.Contains(compose, `"5050:5006"`) {
		t.Errorf("expected port mapping 5050:5006 in compose, got:\n%s", compose)
	}
	if !strings.Contains(compose, "PASSWORD=custompass123") {
		t.Errorf("expected PASSWORD=custompass123 in compose, got:\n%s", compose)
	}
	if !strings.Contains(compose, ":/data") {
		t.Errorf("expected mount :/data in compose, got:\n%s", compose)
	}
	if !strings.Contains(compose, `x-portals:`) {
		t.Errorf("expected x-portals in compose, got:\n%s", compose)
	}
}

func TestGenerateComposeFromQuestions_OnlyPortalsInWebUICandidates(t *testing.T) {
	app := domain.CatalogApp{
		AppID:          "qbittorrent",
		ContainerImage: "ghcr.io/home-operations/qbittorrent:latest",
		DefaultPort:    8080,
		Questions: []domain.CatalogQuestion{
			{
				Variable:     "network.web_port",
				Label:        "WebUI Port",
				Type:         "int",
				DefaultValue: "30024",
			},
			{
				Variable:     "network.bt_port",
				Label:        "BT Port",
				Type:         "int",
				DefaultValue: "51413",
			},
		},
	}

	answers := map[string]string{
		"network.web_port": "30024",
		"network.bt_port":  "51413",
	}

	compose := GenerateComposeFromQuestions(app, answers, "tank", "/mnt/tank/apps")

	// Both ports should be exposed in ports mapping
	if !strings.Contains(compose, `"30024:8080"`) && !strings.Contains(compose, `"30024:30024"`) {
		t.Errorf("expected web port in compose ports, got:\n%s", compose)
	}
	if !strings.Contains(compose, `"51413:51413"`) {
		t.Errorf("expected bt port 51413:51413 in compose ports, got:\n%s", compose)
	}

	// But ONLY web_port should be in x-portals!
	if !strings.Contains(compose, "x-portals:") {
		t.Fatalf("expected x-portals in compose, got:\n%s", compose)
	}
	if !strings.Contains(compose, `name: "WebUI Port"`) || !strings.Contains(compose, `port: 30024`) {
		t.Errorf("expected WebUI Port 30024 in x-portals, got:\n%s", compose)
	}
	if strings.Contains(compose, `name: "BT Port"`) || strings.Contains(compose, `port: 51413`) {
		t.Errorf("BT Port must NOT be exposed as a Web UI candidate portal, got:\n%s", compose)
	}
}

const sampleTrueNASElasticsearchQuestionsYAML = `
groups:
  - name: Elastic Search Configuration
  - name: Network Configuration
  - name: Storage Configuration

questions:
  - variable: storage
    group: Storage Configuration
    schema:
      type: dict
      attrs:
        - variable: data
          label: Elastic Search Data Storage
          description: The path to store Elastic Search Data.
          schema:
            type: dict
            attrs:
              - variable: type
                label: Type
                schema:
                  type: string
                  default: "ix_volume"
                  enum:
                    - value: "host_path"
                    - value: "ix_volume"
              - variable: ix_volume_config
                schema:
                  type: dict
                  attrs:
                    - variable: acl_enable
                      schema:
                        type: boolean
                        default: false
                    - variable: dataset_name
                      schema:
                        type: string
                        default: "data"
              - variable: host_path_config
                schema:
                  type: dict
                  attrs:
                    - variable: host_path
                      schema:
                        type: hostpath
`

const sampleElasticsearchComposeTemplate = `
{% do c1.add_storage("/usr/share/elasticsearch/data", values.storage.data) %}
{% do c1.add_storage("/usr/share/elasticsearch/config/certs", {"type": "anonymous"}) %}
`

func TestParseQuestionsYAML_TrueNASStorageCollapsesToSingleHostPath(t *testing.T) {
	groups, flat := ParseQuestionsYAMLWithTemplate(
		[]byte(sampleTrueNASElasticsearchQuestionsYAML),
		"elastic-search",
		"fast",
		"/mnt/fast/apps",
		sampleElasticsearchComposeTemplate,
	)

	if len(flat) == 0 {
		t.Fatalf("expected parsed questions, got none")
	}

	// Ensure internal TrueNAS settings (type, ix_volume_config, dataset_name) are NOT exposed as questions!
	for _, q := range flat {
		if strings.Contains(q.Variable, "type") || strings.Contains(q.Variable, "acl_enable") || strings.Contains(q.Variable, "dataset_name") {
			t.Errorf("internal TrueNAS plumbing variable %q should not be exposed as a question", q.Variable)
		}
	}

	// Find the collapsed storage question
	var storageQ *domain.CatalogQuestion
	for _, q := range flat {
		if q.Variable == "storage.data" {
			storageQ = &q
			break
		}
	}

	if storageQ == nil {
		t.Fatalf("expected storage.data question to be present, got questions: %+v", flat)
	}

	if storageQ.Type != "hostpath" {
		t.Errorf("expected type hostpath, got %s", storageQ.Type)
	}
	if storageQ.DefaultValue != "/mnt/fast/apps/elastic-search/data" {
		t.Errorf("expected default /mnt/fast/apps/elastic-search/data, got %s", storageQ.DefaultValue)
	}
	if storageQ.MountPath != "/usr/share/elasticsearch/data" {
		t.Errorf("expected mountPath /usr/share/elasticsearch/data, got %s", storageQ.MountPath)
	}

	// Verify group is Persistent Storage & Datasets
	if storageQ.Group != "Persistent Storage & Datasets" {
		t.Errorf("expected group Persistent Storage & Datasets, got %s", storageQ.Group)
	}
	_ = groups
}

func TestExtractStorageMountPaths(t *testing.T) {
	tpl := `
{% do c1.add_storage("/usr/share/elasticsearch/data", values.storage.data) %}
{% do c1.add_storage(values.consts.config_path, values.storage.config) %}
{% do perm_container.add_or_skip_action("config", values.storage.config, perms_config) %}
{% do nc_storage.x.append(("/var/www/html", values.storage.html)) %}
`
	values := `
consts:
  config_path: /config
`
	paths := ExtractStorageMountPaths(tpl, values)
	if paths["data"] != "/usr/share/elasticsearch/data" {
		t.Errorf("expected data -> /usr/share/elasticsearch/data, got %s", paths["data"])
	}
	if paths["config"] != "/config" {
		t.Errorf("expected config -> /config, got %s", paths["config"])
	}
	if paths["html"] != "/var/www/html" {
		t.Errorf("expected html -> /var/www/html, got %s", paths["html"])
	}
}

func TestGenerateComposeFromQuestions_PreservesExistingComposeAndImage(t *testing.T) {
	app := domain.CatalogApp{
		AppID:          "org.icewhale.esphome",
		Title:          "ESPHome",
		ContainerImage: "ghcr.io/esphome/esphome:2026.5.1",
		DefaultPort:    6052,
		ComposeYAML: `x-portals:
  - name: "Web UI"
    scheme: "http"
    port: 6052
    path: "/"
name: esphome
services:
  esphome:
    container_name: esphome
    image: ghcr.io/esphome/esphome:2026.5.1
    restart: always
    ports:
      - "6052:6052"
    volumes:
      - /mnt/fast/apps/esphome/config:/config
    network_mode: host
`,
		Questions: []domain.CatalogQuestion{
			{
				Variable:     "web_port",
				Label:        "Web UI Port",
				Type:         "int",
				DefaultValue: "6052",
			},
			{
				Variable:     "storage.config",
				Label:        "Storage Path (/config)",
				Type:         "hostpath",
				DefaultValue: "/mnt/fast/apps/esphome/config",
				MountPath:    "/config",
			},
		},
	}

	answers := map[string]string{
		"web_port":       "7052",
		"storage.config": "/mnt/fast/apps/esphome/custom-config",
	}

	out := GenerateComposeFromQuestions(app, answers, "fast", "/mnt/fast/apps")

	if strings.Contains(out, "org-icewhale-esphome:latest") {
		t.Errorf("must not generate dummy image org-icewhale-esphome:latest")
	}
	if !strings.Contains(out, "ghcr.io/esphome/esphome:2026.5.1") {
		t.Errorf("expected original image to be preserved, got:\n%s", out)
	}
	if !strings.Contains(out, "network_mode: host") {
		t.Errorf("expected network_mode: host to be preserved, got:\n%s", out)
	}
	if !strings.Contains(out, "port: 7052") {
		t.Errorf("expected x-portals to update to port 7052, got:\n%s", out)
	}
	if !strings.Contains(out, "/mnt/fast/apps/esphome/custom-config:/config") {
		t.Errorf("expected volume to update to /mnt/fast/apps/esphome/custom-config:/config, got:\n%s", out)
	}
}

