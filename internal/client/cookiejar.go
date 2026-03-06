package client

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// persistedCookie is a cookie entry we store in JSON.
type persistedCookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Path     string    `json:"path"`
	Domain   string    `json:"domain"`
	Expires  time.Time `json:"expires"`
	Secure   bool      `json:"secure"`
	HttpOnly bool      `json:"http_only"`
}

// fileJar implements http.CookieJar with persistence to a JSON file.
type fileJar struct {
	mu       sync.Mutex
	file     string
	byHost   map[string][]*persistedCookie
	modified bool
}

// newFileJar creates a persistent cookie jar. If file exists it is loaded.
func newFileJar(file string) (*fileJar, error) {
	j := &fileJar{file: file, byHost: make(map[string][]*persistedCookie)}
	if file == "" {
		return j, nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return nil, err
	}
	if b, err := os.ReadFile(file); err == nil {
		var data struct {
			Cookies map[string][]*persistedCookie `json:"cookies"`
		}
		if jsonErr := json.Unmarshal(b, &data); jsonErr == nil && data.Cookies != nil {
			j.byHost = data.Cookies
		}
	}
	return j, nil
}

func (j *fileJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	now := time.Now()
	host := hostFromURL(u)
	var out []*http.Cookie
	for _, pc := range j.byHost[host] {
		if !pc.Expires.IsZero() && pc.Expires.Before(now) {
			continue
		}
		if !pathMatch(u.Path, pc.Path) {
			continue
		}
		out = append(out, &http.Cookie{
			Name:     pc.Name,
			Value:    pc.Value,
			Path:     pc.Path,
			Domain:   pc.Domain,
			Expires:  pc.Expires,
			Secure:   pc.Secure,
			HttpOnly: pc.HttpOnly,
		})
	}
	return out
}

func (j *fileJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	host := hostFromURL(u)
	existing := j.byHost[host]
	for _, c := range cookies {
		pc := &persistedCookie{
			Name:     c.Name,
			Value:    c.Value,
			Path:     c.Path,
			Domain:   c.Domain,
			Expires:  c.Expires,
			Secure:   c.Secure,
			HttpOnly: c.HttpOnly,
		}
		existing = setOrReplaceCookie(existing, pc)
	}
	j.byHost[host] = existing
	j.modified = true
	if j.file != "" {
		if err := j.saveLocked(); err != nil {
			slog.Warn("cookie jar save failed", "file", j.file, "err", err)
		}
	}
}

func setOrReplaceCookie(list []*persistedCookie, c *persistedCookie) []*persistedCookie {
	for i, x := range list {
		if x.Name == c.Name && x.Path == c.Path {
			list[i] = c
			return list
		}
	}
	return append(list, c)
}

func (j *fileJar) saveLocked() error {
	if !j.modified || j.file == "" {
		return nil
	}
	data := struct {
		Cookies map[string][]*persistedCookie `json:"cookies"`
	}{Cookies: j.byHost}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if err := os.WriteFile(j.file, b, 0600); err != nil {
		return err
	}
	j.modified = false
	return nil
}

func hostFromURL(u *url.URL) string {
	return u.Host
}

func pathMatch(requestPath, cookiePath string) bool {
	if cookiePath == "" || cookiePath == "/" {
		return true
	}
	return len(requestPath) >= len(cookiePath) &&
		(requestPath == cookiePath || requestPath[:len(cookiePath)] == cookiePath &&
			(len(requestPath) == len(cookiePath) || requestPath[len(cookiePath)] == '/'))
}
