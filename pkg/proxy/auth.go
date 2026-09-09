package proxy

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

type Authenticator struct {
	enabled  bool
	username string
	password string
}

func NewAuthenticator(username, password string) *Authenticator {
	u := strings.TrimSpace(username)
	p := strings.TrimSpace(password)
	return &Authenticator{
		enabled:  u != "" && p != "",
		username: u,
		password: p,
	}
}

func (a *Authenticator) IsEnabled() bool {
	return a.enabled
}

func (a *Authenticator) Verify(user, pass string) bool {
	if !a.enabled {
		return true
	}
	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(a.username)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(a.password)) == 1
	return userMatch && passMatch
}

func (a *Authenticator) VerifyHTTP(req *http.Request) bool {
	if !a.enabled {
		return true
	}

	authHeader := req.Header.Get("Proxy-Authorization")
	if authHeader == "" {
		return false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Basic") {
		return false
	}

	payload, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	pair := strings.SplitN(string(payload), ":", 2)
	if len(pair) != 2 {
		return false
	}

	return a.Verify(pair[0], pair[1])
}
