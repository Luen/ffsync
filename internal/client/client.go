package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"
)

// DefaultBaseURL is the default FolderFort server.
const DefaultBaseURL = "https://na.folderfort.com"

// Client is the FolderFort HTTP client (cookies + XSRF).
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	mu         sync.Mutex
}

// New creates a new Client with cookie jar. BaseURL should not have trailing slash.
func New(baseURL string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Jar: jar,
			Transport: &http.Transport{
				// allow cancellation and timeouts
			},
		},
	}, nil
}

func (c *Client) base() string {
	return c.BaseURL
}

// xsrfToken reads XSRF-TOKEN from the cookie jar for the base URL.
func (c *Client) xsrfToken() (string, error) {
	u, err := url.Parse(c.base())
	if err != nil {
		return "", err
	}
	for _, cookie := range c.HTTPClient.Jar.Cookies(u) {
		if cookie.Name == "XSRF-TOKEN" {
			return cookie.Value, nil
		}
	}
	return "", fmt.Errorf("XSRF-TOKEN cookie not found")
}

// authReq sets common headers for authenticated API calls (X-XSRF-TOKEN, Accept, Referer).
func (c *Client) authReq(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base()+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", c.base()+"/")
	req.Header.Set("Origin", c.base())
	token, err := c.xsrfToken()
	if err == nil {
		req.Header.Set("X-XSRF-TOKEN", token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// Login gets the login page (to set XSRF cookie), then POSTs credentials.
func (c *Client) Login(ctx context.Context, email, password string) error {
	// GET login page to obtain XSRF cookie
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+"/login", nil)
	if err != nil {
		return err
	}
	getReq.Header.Set("User-Agent", "ffsync/1.0")
	getReq.Header.Set("Accept", "text/html,application/json")
	resp, err := c.HTTPClient.Do(getReq)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login page: %s", resp.Status)
	}

	token, err := c.xsrfToken()
	if err != nil {
		return fmt.Errorf("no XSRF token after login page: %w", err)
	}

	body := LoginRequest{Email: email, Password: password, Remember: true}
	raw, _ := json.Marshal(body)
	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/auth/login", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Accept", "application/json")
	postReq.Header.Set("X-XSRF-TOKEN", token)
	postReq.Header.Set("Referer", c.base()+"/login")
	postReq.Header.Set("Origin", c.base())
	postReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	postResp, err := c.HTTPClient.Do(postReq)
	if err != nil {
		return err
	}
	defer postResp.Body.Close()
	// Consume body so cookies from redirect responses are applied (e.g. Laravel session)
	_, _ = io.Copy(io.Discard, postResp.Body)
	if postResp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed: %s", postResp.Status)
	}
	// Verify we have session: XSRF is required for subsequent API calls
	if _, err := c.xsrfToken(); err != nil {
		return fmt.Errorf("login succeeded but no XSRF token; session may not have been set")
	}
	return nil
}

// List returns file entries for a folder. Use empty folderID for root.
func (c *Client) List(ctx context.Context, folderID string) ([]FileEntryResponse, error) {
	u, _ := url.Parse(c.base() + "/api/v1/drive/file-entries")
	q := u.Query()
	q.Set("section", "folder")
	q.Set("folderId", folderID)
	q.Set("workspaceId", "0")
	q.Set("orderBy", "updated_at")
	q.Set("orderDir", "desc")
	u.RawQuery = q.Encode()

	req, err := c.authReq(ctx, http.MethodGet, u.Path+"?"+u.RawQuery, nil)
	if err != nil {
		return nil, err
	}
	req.URL = u
	req.Method = http.MethodGet
	req.Body = nil
	req.GetBody = nil

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list failed: %s (%s)", resp.Status, string(bb))
	}
	var out FileEntriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// CreateFolder creates a folder under parentID (empty for root).
func (c *Client) CreateFolder(ctx context.Context, name, parentID string) (string, error) {
	var parent *string
	if parentID != "" {
		parent = &parentID
	}
	body := CreateFolderRequest{Name: name, ParentID: parent}
	raw, _ := json.Marshal(body)
	req, err := c.authReq(ctx, http.MethodPost, "/api/v1/folders", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Referer", c.base()+"/drive/folders/"+parentID)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bb, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create folder failed: %s (%s)", resp.Status, string(bb))
	}
	var out CreateFolderResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Folder.ID.String(), nil
}

// EnsureFolderPath resolves path (e.g. "a/b/c") to a folder ID, creating missing parents.
// baseFolderID is the root folder ID to start from (e.g. from List("")).
func (c *Client) EnsureFolderPath(ctx context.Context, baseFolderID, path string) (string, error) {
	if path == "" || path == "." {
		return baseFolderID, nil
	}
	parts := splitPath(path)
	currentID := baseFolderID
	for _, name := range parts {
		entries, err := c.List(ctx, currentID)
		if err != nil {
			return "", err
		}
		var foundID string
		for _, e := range entries {
			if e.Type == "folder" && e.Name == name {
				foundID = e.ID.String()
				break
			}
		}
		if foundID != "" {
			currentID = foundID
			continue
		}
		// create folder
		id, err := c.CreateFolder(ctx, name, currentID)
		if err != nil {
			return "", err
		}
		currentID = id
	}
	return currentID, nil
}

func splitPath(p string) []string {
	var out []string
	for _, s := range splitSlash(p) {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func splitSlash(p string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(p); i++ {
		if i == len(p) || p[i] == '/' {
			out = append(out, p[start:i])
			start = i + 1
		}
	}
	return out
}

// RootFolderID returns the first folder at root to use as "base" (or creates one named baseName).
// If baseName is empty, returns the first root folder ID if any, else error.
func (c *Client) RootFolderID(ctx context.Context, baseName string) (string, error) {
	entries, err := c.List(ctx, "")
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.Type == "folder" && (baseName == "" || e.Name == baseName) {
			return e.ID.String(), nil
		}
	}
	if baseName == "" {
		return "", fmt.Errorf("no folder at root; specify a base folder name")
	}
	return c.CreateFolder(ctx, baseName, "")
}

// Presign gets a presigned S3 URL for upload.
func (c *Client) Presign(ctx context.Context, filename, mime string, size int64, extension, parentID string) (*PresignResponse, error) {
	body := PresignRequest{
		TotalFileCount: 1,
		Filename:       filename,
		Mime:           mime,
		Disk:           "uploads",
		Size:           size,
		Extension:      extension,
		WorkspaceID:    0,
		ParentID:       parentID,
		RelativePath:   "",
	}
	raw, _ := json.Marshal(body)
	req, err := c.authReq(ctx, http.MethodPost, "/api/v1/s3/simple/presign", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", c.base()+"/drive/folders/"+parentID)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("presign failed: %s (%s)", resp.Status, string(bb))
	}
	var out PresignResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UploadToS3 streams body to the presigned URL with Content-Type, Content-Length, and x-amz-acl.
func (c *Client) UploadToS3(ctx context.Context, presignURL, acl, contentType string, size int64, body io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, presignURL, body)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-amz-acl", acl)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("s3 upload failed: %s (%s)", resp.Status, string(bb))
	}
	return nil
}

// CreateEntry creates the file entry after S3 upload.
func (c *Client) CreateEntry(ctx context.Context, parentID, clientMime, clientName, filename string, size int64, extension string) (string, error) {
	body := CreateEntryRequest{
		WorkspaceID:      0,
		ParentID:         parentID,
		RelativePath:     "",
		Disk:             "uploads",
		ClientMime:       clientMime,
		ClientName:       clientName,
		Filename:         filename,
		Size:             size,
		ClientExtension:  extension,
	}
	raw, _ := json.Marshal(body)
	req, err := c.authReq(ctx, http.MethodPost, "/api/v1/s3/entries", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Referer", c.base()+"/drive/folders/"+parentID)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bb, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create entry failed: %s (%s)", resp.Status, string(bb))
	}
	var out CreateEntryResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.FileEntry.ID.String(), nil
}

// retryWithBackoff runs fn up to maxAttempts with exponential backoff on error.
func retryWithBackoff(ctx context.Context, maxAttempts int, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return lastErr
}

const defaultMaxRetries = 3

// ListRecursive returns all files and folders under folderID with paths relative to prefix.
// prefix is the path prefix for the current folder (e.g. "" or "a/b").
func (c *Client) ListRecursive(ctx context.Context, folderID, prefix string) (files map[string]FileEntryResponse, folders map[string]string, err error) {
	files = make(map[string]FileEntryResponse)
	folders = make(map[string]string)
	entries, err := c.List(ctx, folderID)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		fullPath := e.Name
		if prefix != "" {
			fullPath = prefix + "/" + e.Name
		}
		if e.Type == "folder" {
			folders[fullPath] = e.ID.String()
			subFiles, subFolders, err := c.ListRecursive(ctx, e.ID.String(), fullPath)
			if err != nil {
				return nil, nil, err
			}
			for k, v := range subFiles {
				files[k] = v
			}
			for k, v := range subFolders {
				folders[k] = v
			}
		} else {
			files[fullPath] = e
		}
	}
	return files, folders, nil
}

// DeleteFileEntry deletes a file or folder by ID.
func (c *Client) DeleteFileEntry(ctx context.Context, entryID string) error {
	req, err := c.authReq(ctx, http.MethodDelete, "/api/v1/file-entries/"+entryID, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed: %s (%s)", resp.Status, string(bb))
	}
	return nil
}

// UploadFile performs presign, streaming PUT, and create entry with retries.
// name is the base filename; body is read to completion (size bytes).
func (c *Client) UploadFile(ctx context.Context, parentID, name, mime, extension string, size int64, body io.Reader) (entryID string, err error) {
	var pres *PresignResponse
	err = retryWithBackoff(ctx, defaultMaxRetries, func() error {
		var e error
		pres, e = c.Presign(ctx, name, mime, size, extension, parentID)
		return e
	})
	if err != nil {
		return "", err
	}
	err = retryWithBackoff(ctx, defaultMaxRetries, func() error {
		return c.UploadToS3(ctx, pres.URL, pres.ACL, mime, size, body)
	})
	if err != nil {
		return "", err
	}
	// filename for entry is the last part of pres.Key
	filename := pres.Key
	if i := len(pres.Key) - 1; i >= 0 {
		for j := i; j >= 0; j-- {
			if pres.Key[j] == '/' {
				filename = pres.Key[j+1:]
				break
			}
		}
	}
	err = retryWithBackoff(ctx, defaultMaxRetries, func() error {
		var e error
		entryID, e = c.CreateEntry(ctx, parentID, mime, name, filename, size, extension)
		return e
	})
	return entryID, err
}
