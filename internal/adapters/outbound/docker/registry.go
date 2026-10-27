package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
)

// Registry credentials are kept in a Docker CLI compatible config.json inside the NAS system
// dataset (on the pool, not the boot disk). Every docker / docker compose invocation made by the
// adapter points DOCKER_CONFIG at this directory, so pulls are authenticated without relying on
// root's ~/.docker. A sidecar registries.json stores non-secret metadata for the UI.

const (
	registryAuthDirName  = ".registry-auth"
	registryMetaFileName = "registries.json"
	registryLoginTimeout = 45 * time.Second
)

type registryRecord struct {
	ID              string    `json:"id"`
	Server          string    `json:"server"`
	Username        string    `json:"username"`
	Description     string    `json:"description,omitempty"`
	LastLoginStatus string    `json:"lastLoginStatus,omitempty"`
	LastLoginTime   time.Time `json:"lastLoginTime,omitempty"`
}

type dockerAuthEntry struct {
	Auth string `json:"auth,omitempty"`
}

// registryConfigDir returns (and creates with 0700) the DOCKER_CONFIG directory used for all
// docker CLI invocations.
func (a *Adapter) registryConfigDir() string {
	dir := filepath.Join(a.unitsDir, registryAuthDirName)
	_ = os.MkdirAll(dir, 0o700)
	_ = os.Chmod(dir, 0o700)
	return dir
}

// dockerCLIEnv returns the environment for docker / docker compose subprocesses.
func (a *Adapter) dockerCLIEnv() []string {
	env := os.Environ()
	if a.socketPath != "" {
		env = append(env, fmt.Sprintf("DOCKER_HOST=unix://%s", a.socketPath))
	}
	if a.unitsDir != "" {
		env = append(env, "DOCKER_CONFIG="+a.registryConfigDir())
	}
	return env
}

func (a *Adapter) readRegistryMeta() ([]registryRecord, error) {
	data, err := os.ReadFile(filepath.Join(a.registryConfigDir(), registryMetaFileName))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recs []registryRecord
	if err := json.Unmarshal(data, &recs); err != nil {
		return nil, fmt.Errorf("corrupt %s: %w", registryMetaFileName, err)
	}
	return recs, nil
}

func (a *Adapter) writeRegistryMeta(recs []registryRecord) error {
	sort.Slice(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })
	data, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(a.registryConfigDir(), registryMetaFileName), data, 0o600)
}

// readDockerConfig returns the raw top-level config.json map plus the decoded auths section.
func (a *Adapter) readDockerConfig() (map[string]json.RawMessage, map[string]dockerAuthEntry, error) {
	top := map[string]json.RawMessage{}
	auths := map[string]dockerAuthEntry{}
	data, err := os.ReadFile(filepath.Join(a.registryConfigDir(), "config.json"))
	if os.IsNotExist(err) {
		return top, auths, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, nil, fmt.Errorf("corrupt docker config.json: %w", err)
	}
	if raw, ok := top["auths"]; ok {
		_ = json.Unmarshal(raw, &auths)
	}
	return top, auths, nil
}

func (a *Adapter) writeDockerConfig(top map[string]json.RawMessage, auths map[string]dockerAuthEntry) error {
	raw, err := json.Marshal(auths)
	if err != nil {
		return err
	}
	top["auths"] = raw
	data, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(a.registryConfigDir(), "config.json"), data, 0o600)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func encodeBasicAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}

func decodeBasicAuth(auth string) (user, pass string, ok bool) {
	raw, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return "", "", false
	}
	user, pass, ok = strings.Cut(string(raw), ":")
	return user, pass, ok
}

func registryIDFromServer(server string) string {
	id := domain.ToZfsSafeSlug(strings.ReplaceAll(domain.NormalizeRegistryServer(server), ":", "-"))
	if id == "" {
		id = "registry"
	}
	return id
}

func recordToDomain(r registryRecord, hasPassword bool) domain.DockerRegistry {
	return domain.DockerRegistry{
		ID:              r.ID,
		Server:          r.Server,
		Username:        r.Username,
		HasPassword:     hasPassword,
		Description:     r.Description,
		LastLoginStatus: r.LastLoginStatus,
		LastLoginTime:   r.LastLoginTime,
	}
}

// verifyRegistryLogin runs `docker login` against a throwaway config dir so a bad password never
// replaces working stored credentials.
func (a *Adapter) verifyRegistryLogin(ctx context.Context, server, user, pass string) error {
	if a.registryVerify != nil {
		return a.registryVerify(ctx, server, user, pass)
	}
	tmp, err := os.MkdirTemp("", "nasctl-registry-login-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	lctx, cancel := context.WithTimeout(ctx, registryLoginTimeout)
	defer cancel()
	cmd := exec.CommandContext(lctx, "docker", "--config", tmp, "login", "--username", user, "--password-stdin", domain.RegistryAuthKey(server))
	if a.socketPath != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("DOCKER_HOST=unix://%s", a.socketPath))
	}
	cmd.Stdin = strings.NewReader(pass)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%w: login to %s failed: %s", domain.ErrInvalidArgument, server, msg)
	}
	return nil
}

// ListRegistries returns stored registry credentials (without passwords).
func (a *Adapter) ListRegistries(ctx context.Context) ([]domain.DockerRegistry, error) {
	a.registryMu.Lock()
	defer a.registryMu.Unlock()
	recs, err := a.readRegistryMeta()
	if err != nil {
		return nil, err
	}
	_, auths, err := a.readDockerConfig()
	if err != nil {
		return nil, err
	}
	out := make([]domain.DockerRegistry, 0, len(recs))
	for _, r := range recs {
		_, has := auths[domain.RegistryAuthKey(r.Server)]
		out = append(out, recordToDomain(r, has))
	}
	return out, nil
}

// GetRegistry returns one stored registry by ID.
func (a *Adapter) GetRegistry(ctx context.Context, id string) (*domain.DockerRegistry, error) {
	list, err := a.ListRegistries(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range list {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("%w: docker registry %q", domain.ErrNotFound, id)
}

// CreateRegistry verifies and stores credentials for a registry.
func (a *Adapter) CreateRegistry(ctx context.Context, reg domain.DockerRegistry) (*domain.DockerRegistry, error) {
	reg.Server = domain.NormalizeRegistryServer(reg.Server)
	reg.Username = strings.TrimSpace(reg.Username)
	if err := reg.Validate(); err != nil {
		return nil, err
	}
	if reg.Password == "" {
		return nil, fmt.Errorf("%w: registry password or access token is required", domain.ErrInvalidArgument)
	}
	if reg.ID == "" {
		reg.ID = registryIDFromServer(reg.Server)
	}
	if err := domain.ValidateResourceID(reg.ID, "docker_registry"); err != nil {
		return nil, err
	}

	a.registryMu.Lock()
	recs, err := a.readRegistryMeta()
	a.registryMu.Unlock()
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		if r.ID == reg.ID {
			return nil, fmt.Errorf("%w: docker registry %q", domain.ErrAlreadyExists, reg.ID)
		}
		if r.Server == reg.Server {
			return nil, fmt.Errorf("%w: credentials for %s already exist (registry %q); edit that entry instead", domain.ErrAlreadyExists, reg.Server, r.ID)
		}
	}

	if err := a.verifyRegistryLogin(ctx, reg.Server, reg.Username, reg.Password); err != nil {
		return nil, err
	}

	a.registryMu.Lock()
	defer a.registryMu.Unlock()
	top, auths, err := a.readDockerConfig()
	if err != nil {
		return nil, err
	}
	auths[domain.RegistryAuthKey(reg.Server)] = dockerAuthEntry{Auth: encodeBasicAuth(reg.Username, reg.Password)}
	if err := a.writeDockerConfig(top, auths); err != nil {
		return nil, err
	}
	rec := registryRecord{ID: reg.ID, Server: reg.Server, Username: reg.Username, Description: reg.Description, LastLoginStatus: "ok", LastLoginTime: time.Now().UTC()}
	recs, _ = a.readRegistryMeta()
	if err := a.writeRegistryMeta(append(recs, rec)); err != nil {
		return nil, err
	}
	a.logger.Info("docker registry credentials saved", "subsystem", "docker", "registry", reg.Server, "username", reg.Username)
	out := recordToDomain(rec, true)
	return &out, nil
}

// UpdateRegistry changes stored credentials. An empty password keeps the current one. Any change to
// server, username or password is verified with `docker login` before it is persisted.
func (a *Adapter) UpdateRegistry(ctx context.Context, reg domain.DockerRegistry, fields []string) (*domain.DockerRegistry, error) {
	has := func(f string) bool {
		if len(fields) == 0 {
			return true
		}
		for _, x := range fields {
			if x == f {
				return true
			}
		}
		return false
	}

	a.registryMu.Lock()
	recs, err := a.readRegistryMeta()
	var _, auths, cfgErr = a.readDockerConfig()
	a.registryMu.Unlock()
	if err != nil {
		return nil, err
	}
	if cfgErr != nil {
		return nil, cfgErr
	}
	idx := -1
	for i, r := range recs {
		if r.ID == reg.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%w: docker registry %q", domain.ErrNotFound, reg.ID)
	}
	cur := recs[idx]
	_, curPass, _ := decodeBasicAuth(auths[domain.RegistryAuthKey(cur.Server)].Auth)

	next := cur
	if has("server") && strings.TrimSpace(reg.Server) != "" {
		next.Server = domain.NormalizeRegistryServer(reg.Server)
	}
	if has("username") && strings.TrimSpace(reg.Username) != "" {
		next.Username = strings.TrimSpace(reg.Username)
	}
	if has("description") {
		next.Description = reg.Description
	}
	pass := curPass
	if has("password") && reg.Password != "" {
		pass = reg.Password
	}
	if next.Server != cur.Server {
		for _, r := range recs {
			if r.ID != cur.ID && r.Server == next.Server {
				return nil, fmt.Errorf("%w: credentials for %s already exist (registry %q)", domain.ErrAlreadyExists, next.Server, r.ID)
			}
		}
	}

	credsChanged := next.Server != cur.Server || next.Username != cur.Username || pass != curPass
	if credsChanged {
		if pass == "" {
			return nil, fmt.Errorf("%w: registry password or access token is required", domain.ErrInvalidArgument)
		}
		if err := a.verifyRegistryLogin(ctx, next.Server, next.Username, pass); err != nil {
			return nil, err
		}
		next.LastLoginStatus = "ok"
		next.LastLoginTime = time.Now().UTC()
	}

	a.registryMu.Lock()
	defer a.registryMu.Unlock()
	top, auths, err := a.readDockerConfig()
	if err != nil {
		return nil, err
	}
	if credsChanged {
		delete(auths, domain.RegistryAuthKey(cur.Server))
		auths[domain.RegistryAuthKey(next.Server)] = dockerAuthEntry{Auth: encodeBasicAuth(next.Username, pass)}
		if err := a.writeDockerConfig(top, auths); err != nil {
			return nil, err
		}
	}
	recs, _ = a.readRegistryMeta()
	for i := range recs {
		if recs[i].ID == cur.ID {
			recs[i] = next
		}
	}
	if err := a.writeRegistryMeta(recs); err != nil {
		return nil, err
	}
	_, hasAuth := auths[domain.RegistryAuthKey(next.Server)]
	out := recordToDomain(next, hasAuth)
	return &out, nil
}

// DeleteRegistry removes stored credentials for a registry.
func (a *Adapter) DeleteRegistry(ctx context.Context, id string) error {
	a.registryMu.Lock()
	defer a.registryMu.Unlock()
	recs, err := a.readRegistryMeta()
	if err != nil {
		return err
	}
	var kept []registryRecord
	var removed *registryRecord
	for i := range recs {
		if recs[i].ID == id {
			removed = &recs[i]
			continue
		}
		kept = append(kept, recs[i])
	}
	if removed == nil {
		return fmt.Errorf("%w: docker registry %q", domain.ErrNotFound, id)
	}
	top, auths, err := a.readDockerConfig()
	if err != nil {
		return err
	}
	delete(auths, domain.RegistryAuthKey(removed.Server))
	if err := a.writeDockerConfig(top, auths); err != nil {
		return err
	}
	if kept == nil {
		kept = []registryRecord{}
	}
	return a.writeRegistryMeta(kept)
}

// registryAuthHeader returns the X-Registry-Auth header value for pulling image through the Engine
// API, or "" when no credentials are stored for its registry.
func (a *Adapter) registryAuthHeader(image string) string {
	if a.unitsDir == "" {
		return ""
	}
	server := domain.RegistryForImage(image)
	a.registryMu.Lock()
	_, auths, err := a.readDockerConfig()
	a.registryMu.Unlock()
	if err != nil {
		return ""
	}
	entry, ok := auths[domain.RegistryAuthKey(server)]
	if !ok {
		return ""
	}
	user, pass, ok := decodeBasicAuth(entry.Auth)
	if !ok {
		return ""
	}
	payload, _ := json.Marshal(map[string]string{"username": user, "password": pass, "serveraddress": domain.RegistryAuthKey(server)})
	return base64.URLEncoding.EncodeToString(payload)
}

// seedRegistriesFromHome imports credentials from root's existing ~/.docker/config.json the first
// time the NAS registry store is initialised, so earlier manual `docker login`s keep working.
func (a *Adapter) seedRegistriesFromHome() {
	if a.unitsDir == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(a.registryConfigDir(), registryMetaFileName)); err == nil {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(home, ".docker", "config.json"))
	if err != nil {
		_ = a.writeRegistryMeta([]registryRecord{})
		return
	}
	var cfg struct {
		Auths map[string]dockerAuthEntry `json:"auths"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		_ = a.writeRegistryMeta([]registryRecord{})
		return
	}
	top, auths, err := a.readDockerConfig()
	if err != nil {
		return
	}
	var recs []registryRecord
	seen := map[string]bool{}
	for key, e := range cfg.Auths {
		user, _, ok := decodeBasicAuth(e.Auth)
		if !ok || user == "" {
			continue // credential-helper entries carry no inline secret
		}
		server := domain.NormalizeRegistryServer(key)
		if seen[server] {
			continue
		}
		seen[server] = true
		auths[domain.RegistryAuthKey(server)] = e
		recs = append(recs, registryRecord{ID: registryIDFromServer(server), Server: server, Username: user, Description: "Imported from " + filepath.Join(home, ".docker", "config.json"), LastLoginStatus: "imported (not verified)"})
	}
	if len(recs) > 0 {
		if a.writeDockerConfig(top, auths) == nil {
			a.logger.Info("imported docker registry credentials", "subsystem", "docker", "count", len(recs))
		}
	}
	if recs == nil {
		recs = []registryRecord{}
	}
	_ = a.writeRegistryMeta(recs)
}
