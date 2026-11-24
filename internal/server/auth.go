package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	passwdFile = "passwd"
	sessionKey = "session.key"
	sessionTTL = 7 * 24 * time.Hour
)

// Auth manages the single administrator password (bcrypt) and HMAC-signed
// session tokens persisted across restarts via session.key.
type Auth struct {
	mu     sync.Mutex
	secret []byte
	dir    string
}

func NewAuth(dataDir string) (*Auth, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	a := &Auth{dir: dataDir}
	path := filepath.Join(dataDir, sessionKey)
	data, err := os.ReadFile(path)
	if err == nil && len(data) >= 32 {
		a.secret = data
		return a, nil
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, secret, 0o600); err != nil {
		return nil, err
	}
	a.secret = secret
	return a, nil
}

func (a *Auth) passwdPath() string { return filepath.Join(a.dir, passwdFile) }

// Initialized reports whether the admin password has been set up.
func (a *Auth) Initialized() bool {
	_, err := os.Stat(a.passwdPath())
	return err == nil
}

func (a *Auth) Initialize(password string) error {
	if a.Initialized() {
		return fmt.Errorf("已初始化")
	}
	return a.writePasswd(password)
}

func (a *Auth) writePasswd(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return err
	}
	return os.WriteFile(a.passwdPath(), hash, 0o600)
}

// Verify checks the admin password.
func (a *Auth) Verify(password string) bool {
	hash, err := os.ReadFile(a.passwdPath())
	if err != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
}

func (a *Auth) ChangePassword(current, next string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.Verify(current) {
		return fmt.Errorf("当前密码错误")
	}
	return a.writePasswd(next)
}

// Token issues a signed session token: <expiryHex>.<hmac>.
func (a *Auth) Token() string {
	exp := time.Now().Add(sessionTTL).Unix()
	payload := strconv.FormatInt(exp, 16)
	return payload + "." + a.sign(payload)
}

func (a *Auth) Check(token string) bool {
	payload, sig, ok := split2(token, ".")
	if !ok {
		return false
	}
	if hmac.Equal([]byte(sig), []byte(a.sign(payload))) != true {
		return false
	}
	exp, err := strconv.ParseInt(payload, 16, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return true
}

func (a *Auth) sign(payload string) string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte("wrtai-session:" + payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func split2(s, sep string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == sep[0] {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}
