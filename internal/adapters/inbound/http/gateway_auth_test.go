package httpadapter_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestGatewayRejectsUnauthenticatedRequests(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	client := srv.Client()

	for _, path := range []string{
		"/v1/zfsPools",
		"/v1/auth:me",
		"/v1/dockerApps",
		"/v1/events",
		"/events",
		"/v1/dockerApps:watch",
		"/v1/resources:watch",
		"/v1/system:telemetryStream",
		"/v1/storage/isos",
		"/v1/storage/lxc-templates",
	} {
		resp, err := client.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s without token: expected 401, got %d", path, resp.StatusCode)
		}
	}

	resp, err := client.Post(srv.URL+"/v1/dockerApps?dockerAppId=evil", "application/json",
		strings.NewReader(`{"composeYaml":"services: {}"}`))
	if err != nil {
		t.Fatalf("POST dockerApps: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST /v1/dockerApps without token: expected 401, got %d", resp.StatusCode)
	}

	// Query-string tokens are only honoured on GET (EventSource / WebSocket).
	token := login(t, srv)
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/storage/isos?filename=x.iso&access_token="+token, nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("DELETE isos: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("DELETE with access_token query param: expected 401, got %d", resp.StatusCode)
	}
}

func TestGatewayLoginRejectsWrongPassword(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	for _, password := range []string{"admin123", "wrong", ""} {
		payload := fmt.Sprintf(`{"username":%q,"password":%q}`, testLoginUsername(), password)
		resp, err := srv.Client().Post(srv.URL+"/v1/auth:login", "application/json", strings.NewReader(payload))
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("login with password %q unexpectedly succeeded", password)
		}
	}
}

func TestGatewayRejectsTokenSignedWithLegacyDefaultSecret(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	enc := base64.RawURLEncoding
	unsigned := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." +
		enc.EncodeToString([]byte(fmt.Sprintf(`{"sub":%q,"adm":true,"exp":%d}`, testLoginUsername(), time.Now().Add(time.Hour).Unix())))
	mac := hmac.New(sha256.New, []byte("nas-production-secret-2026"))
	mac.Write([]byte(unsigned))
	forged := unsigned + "." + enc.EncodeToString(mac.Sum(nil))

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/zfsPools", nil)
	req.Header.Set("Authorization", "Bearer "+forged)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET zfsPools: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("forged token: expected 401, got %d", resp.StatusCode)
	}
}

func TestGatewaySendsNoCORSHeaders(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	token := login(t, srv)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/zfsPools", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET zfsPools: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if v := resp.Header.Get("Access-Control-Allow-Origin"); v != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header, got %q", v)
	}
}

func TestGatewayWebSocketChecksTokenAndOrigin(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	token := login(t, srv)
	base := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/docker/terminal/c1?shell=/bin/sh"

	if conn, err := websocket.Dial(base, "", srv.URL); err == nil {
		conn.Close()
		t.Errorf("terminal websocket without token unexpectedly connected")
	}
	if conn, err := websocket.Dial(base+"&access_token="+token, "", "https://evil.example"); err == nil {
		conn.Close()
		t.Errorf("terminal websocket from foreign origin unexpectedly connected")
	}
	conn, err := websocket.Dial(base+"&access_token="+token, "", srv.URL)
	if err != nil {
		t.Fatalf("same-origin authenticated terminal websocket failed: %v", err)
	}
	conn.Close()
}

func TestGatewayRejectsUnsafeIDs(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	client := &http.Client{Transport: &bearerTransport{token: login(t, srv), base: srv.Client().Transport}}

	cases := []struct {
		name, path, body string
	}{
		{"dot-dot app id", "/v1/dockerApps?dockerAppId=..", `{"composeYaml":"services: {}"}`},
		{"flag-like app id", "/v1/dockerApps?dockerAppId=-x", `{"composeYaml":"services: {}"}`},
		{"app id from resource name", "/v1/dockerApps", `{"name":"dockerApps/..","composeYaml":"services: {}"}`},
		{"newline in container id", "/v1/lxcContainers?lxcContainerId=a%0Ab", `{}`},
		{"traversal in nested dataset", "/v1/zfsPools/tank/datasets?zfsDatasetId=media/../../etc", `{}`},
		{"flag-like snapshot name", "/v1/zfsPools/tank/datasets/media:snapshot", `{"snapshotName":"-r"}`},
		{"flag-like pool import", "/v1/zfsPools:import", `{"poolName":"-a"}`},
	}
	for _, tc := range cases {
		resp, err := client.Post(srv.URL+tc.path, "application/json", strings.NewReader(tc.body))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", tc.name, resp.StatusCode)
		}
	}
}
