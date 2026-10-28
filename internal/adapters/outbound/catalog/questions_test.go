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
