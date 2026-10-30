package application

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type IdentityService struct {
	identity  ports.LinuxIdentityPort
	jwtSecret []byte
}

// NewIdentityService signs tokens with jwtSecret. An empty secret gets a random
// per-process key, so tokens are never signed with a guessable value.
func NewIdentityService(identity ports.LinuxIdentityPort, jwtSecret string) *IdentityService {
	secret := []byte(jwtSecret)
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			panic(fmt.Sprintf("identity: generating JWT secret: %v", err))
		}
	}
	return &IdentityService{
		identity:  identity,
		jwtSecret: secret,
	}
}

type jwtClaims struct {
	Sub     string `json:"sub"`
	IsAdmin bool   `json:"adm"`
	Exp     int64  `json:"exp"`
	Iat     int64  `json:"iat"`
}

func (s *IdentityService) Login(ctx context.Context, username, password string) (string, int64, *domain.User, error) {
	if strings.TrimSpace(username) == "" || password == "" {
		return "", 0, nil, fmt.Errorf("%w: username and password are required", domain.ErrInvalidArgument)
	}
	user, err := s.identity.Authenticate(ctx, username, password)
	if err != nil {
		return "", 0, nil, err
	}
	expiresIn := int64(86400) // 24h
	now := time.Now().Unix()
	token, err := s.signJWT(jwtClaims{
		Sub:     user.Username,
		IsAdmin: user.IsAdmin,
		Iat:     now,
		Exp:     now + expiresIn,
	})
	if err != nil {
		return "", 0, nil, err
	}
	return token, expiresIn, user, nil
}

func (s *IdentityService) VerifyToken(ctx context.Context, token string) (*domain.User, error) {
	token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: malformed token", domain.ErrUnauthorized)
	}
	unsigned := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, s.jwtSecret)
	mac.Write([]byte(unsigned))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expectedSig), []byte(parts[2])) {
		return nil, fmt.Errorf("%w: invalid token signature", domain.ErrUnauthorized)
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid token payload", domain.ErrUnauthorized)
	}
	var claims jwtClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("%w: invalid token claims", domain.ErrUnauthorized)
	}
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("%w: token expired", domain.ErrUnauthorized)
	}
	return s.identity.GetUser(ctx, claims.Sub)
}

func (s *IdentityService) signJWT(claims jwtClaims) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	unsigned := header + "." + payload
	mac := hmac.New(sha256.New, s.jwtSecret)
	mac.Write([]byte(unsigned))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + sig, nil
}

func (s *IdentityService) ListUsers(ctx context.Context, includeSystem bool) ([]domain.User, error) {
	return s.identity.ListUsers(ctx, includeSystem)
}

func (s *IdentityService) GetUser(ctx context.Context, username string) (*domain.User, error) {
	if err := domain.ValidateResourceID(username, "user"); err != nil {
		return nil, err
	}
	return s.identity.GetUser(ctx, username)
}

func (s *IdentityService) CreateUser(ctx context.Context, user domain.User) (*domain.User, error) {
	user.Username = user.ID
	if err := user.Validate(); err != nil {
		return nil, err
	}
	return s.identity.CreateUser(ctx, user)
}

func (s *IdentityService) UpdateUser(ctx context.Context, user domain.User, fields []string) (*domain.User, error) {
	user.Username = user.ID
	if err := user.Validate(); err != nil {
		return nil, err
	}
	return s.identity.UpdateUser(ctx, user, fields)
}

func (s *IdentityService) DeleteUser(ctx context.Context, username string, removeHome bool) error {
	if err := domain.ValidateResourceID(username, "user"); err != nil {
		return err
	}
	if username == "root" {
		return fmt.Errorf("%w: cannot delete root account", domain.ErrFailedPrecondition)
	}
	return s.identity.DeleteUser(ctx, username, removeHome)
}

func (s *IdentityService) ListGroups(ctx context.Context, includeSystem bool) ([]domain.Group, error) {
	return s.identity.ListGroups(ctx, includeSystem)
}

func (s *IdentityService) GetGroup(ctx context.Context, groupName string) (*domain.Group, error) {
	if err := domain.ValidateResourceID(groupName, "group"); err != nil {
		return nil, err
	}
	return s.identity.GetGroup(ctx, groupName)
}

func (s *IdentityService) CreateGroup(ctx context.Context, group domain.Group) (*domain.Group, error) {
	group.GroupName = group.ID
	if err := group.Validate(); err != nil {
		return nil, err
	}
	return s.identity.CreateGroup(ctx, group)
}

func (s *IdentityService) UpdateGroup(ctx context.Context, group domain.Group, fields []string) (*domain.Group, error) {
	group.GroupName = group.ID
	if err := group.Validate(); err != nil {
		return nil, err
	}
	return s.identity.UpdateGroup(ctx, group, fields)
}

func (s *IdentityService) DeleteGroup(ctx context.Context, groupName string) error {
	if err := domain.ValidateResourceID(groupName, "group"); err != nil {
		return err
	}
	if groupName == "root" || groupName == "sudo" || groupName == "wheel" {
		return fmt.Errorf("%w: cannot delete system privileged group %q", domain.ErrFailedPrecondition, groupName)
	}
	return s.identity.DeleteGroup(ctx, groupName)
}
