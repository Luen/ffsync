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
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the default FolderFort server.
const DefaultBaseURL = "https://na.folderfort.com"

// ListRecurseConcurrency limits concurrent folder listings during ListRecursive (startup).
const ListRecurseConcurrency = 12

// Client is the FolderFort HTTP client (cookies + XSRF).
type Client struct {
	BaseURL        string
	HTTPClient     *http.Client
	mu             sync.Mutex
	folderCache    sync.Map // "parentID\x00name" -> folderID string
	folderLocks    sync.Map // "parentID\x00name" -> *sync.Mutex
	listRecurseSem chan struct{} // limits concurrent ListRecursive subfolder listing
}

// New creates a new Client. If cookieFile is non-empty, cookies are loaded from and
// saved to that file; otherwise an in-memory jar is used (no persistence).
// BaseURL should not have trailing slash.
func New(baseURL string, cookieFile string) (*Client, error) {
	var jar http.CookieJar
	if cookieFile != "" {
		fj, err := newFileJar(cookieFile)
		if err != nil {
			return nil, err
		}
		jar = fj
	} else {
		var err error
		jar, err = cookiejar.New(nil)
		if err != nil {
			return nil, err
		}
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Jar:     jar,
			Timeout: 20 * time.Second,
			Transport: &http.Transport{
				// allow cancellation and timeouts
			},
		},
		listRecurseSem: make(chan struct{}, ListRecurseConcurrency),
	}, nil
}

func (c *Client) base() string {
	return c.BaseURL
}

// folderIDForList returns the folder id to use in list API (file-entries?folderId=). Cache stores "numericId,hash"; list API expects hash. Web uses "0" for root.
func folderIDForList(v string) string {
	if v == "" {
		return "0"
	}
	if i := strings.Index(v, ","); i >= 0 {
		return v[i+1:]
	}
	return v
}

// folderIDForCreate returns the folder id to use in create/presign/entry APIs (parentId in body). Cache stores "numericId,hash"; create API expects integer.
func folderIDForCreate(v string) string {
	if i := strings.Index(v, ","); i >= 0 {
		return v[:i]
	}
	return v
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

// List returns the first page of file entries for a folder (same request as web: no per_page, server returns 50).
func (c *Client) List(ctx context.Context, folderID string) ([]FileEntryResponse, error) {
	entries, _, err := c.ListPage(ctx, folderID, 1, 0)
	return entries, err
}

// ListPage returns one page of file entries. page is 1-based; perPage is the page size (omit 0 to match web: no per_page in URL, server defaults to 50).
// Returns (entries, lastPage). If the API does not return pagination meta, lastPage is 1.
// folderID can be cache value "numericId,hash"; list API expects hash.
func (c *Client) ListPage(ctx context.Context, folderID string, page, perPage int) ([]FileEntryResponse, int, error) {
	listID := folderIDForList(folderID)
	u, _ := url.Parse(c.base() + "/api/v1/drive/file-entries")
	q := u.Query()
	q.Set("section", "folder")
	q.Set("folderId", listID)
	q.Set("workspaceId", "0")
	q.Set("orderBy", "updated_at")
	q.Set("orderDir", "desc")
	q.Set("page", strconv.Itoa(page))
	if perPage > 0 {
		q.Set("per_page", strconv.Itoa(perPage))
	}
	u.RawQuery = q.Encode()

	req, err := c.authReq(ctx, http.MethodGet, u.Path+"?"+u.RawQuery, nil)
	if err != nil {
		return nil, 1, err
	}
	req.URL = u
	req.Method = http.MethodGet
	req.Body = nil
	req.GetBody = nil
	// Match web app: Referer /drive for root (folderId=0), /drive/folders/{id} otherwise.
	if listID == "0" {
		req.Header.Set("Referer", c.base()+"/drive")
	} else {
		req.Header.Set("Referer", c.base()+"/drive/folders/"+listID)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, 1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bb, _ := io.ReadAll(resp.Body)
		return nil, 1, fmt.Errorf("list failed: %s (%s)", resp.Status, string(bb))
	}
	var out FileEntriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 1, err
	}
	lastPage := 1
	if out.Meta != nil && out.Meta.LastPage != nil && *out.Meta.LastPage > 0 {
		lastPage = *out.Meta.LastPage
	}
	return out.Data, lastPage, nil
}

// ListAll returns all file entries for a folder, fetching every page so no existing folder is missed.
// Uses same query as web app (no per_page in URL; server defaults to 50) and Referer /drive/folders/{folderId}.
// Continues until a partial page is received; some APIs report last_page=1 even when more pages exist.
func (c *Client) ListAll(ctx context.Context, folderID string) ([]FileEntryResponse, error) {
	const defaultPageSize = 50 // server default when per_page omitted
	var all []FileEntryResponse
	page := 1
	for {
		entries, _, err := c.ListPage(ctx, folderID, page, 0) // 0 = omit per_page to match web
		if err != nil {
			return nil, err
		}
		all = append(all, entries...)
		// Stop only when we got no entries or a partial page (no more pages). Do not trust lastPage.
		if len(entries) == 0 || len(entries) < defaultPageSize {
			break
		}
		page++
	}
	return all, nil
}

// listPageByName is like ListPage but sorted by name (ascending) for stable pagination.
func (c *Client) listPageByName(ctx context.Context, folderID string, page, perPage int) ([]FileEntryResponse, int, error) {
	listID := folderIDForList(folderID)
	u, _ := url.Parse(c.base() + "/api/v1/drive/file-entries")
	q := u.Query()
	q.Set("section", "folder")
	q.Set("folderId", listID)
	q.Set("workspaceId", "0")
	q.Set("orderBy", "name")
	q.Set("orderDir", "asc")
	q.Set("page", strconv.Itoa(page))
	q.Set("per_page", strconv.Itoa(perPage))
	u.RawQuery = q.Encode()

	req, err := c.authReq(ctx, http.MethodGet, u.Path+"?"+u.RawQuery, nil)
	if err != nil {
		return nil, 1, err
	}
	req.URL = u
	req.Method = http.MethodGet
	req.Body = nil
	req.GetBody = nil
	if listID == "0" {
		req.Header.Set("Referer", c.base()+"/drive")
	} else {
		req.Header.Set("Referer", c.base()+"/drive/folders/"+listID)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, 1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bb, _ := io.ReadAll(resp.Body)
		return nil, 1, fmt.Errorf("list failed: %s (%s)", resp.Status, string(bb))
	}
	var out FileEntriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 1, err
	}
	lastPage := 1
	if out.Meta != nil && out.Meta.LastPage != nil && *out.Meta.LastPage > 0 {
		lastPage = *out.Meta.LastPage
	}
	return out.Data, lastPage, nil
}

// CreateFolder creates a folder under parentID (empty for root).
// parentID can be cache value "numericId,hash"; create API expects integer parentId in body.
// If the API returns 422 "Folder with same name already exists", looks up the existing folder and returns its ID (or "id,hash").
func (c *Client) CreateFolder(ctx context.Context, name, parentID string) (string, error) {
	parentIdForBody := folderIDForCreate(parentID)
	var parent *string
	if parentIdForBody != "" {
		parent = &parentIdForBody
	}
	body := CreateFolderRequest{Name: name, ParentID: parent}
	raw, _ := json.Marshal(body)
	req, err := c.authReq(ctx, http.MethodPost, "/api/v1/folders", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	// Match web app: Referer /drive for root, /drive/folders/{id} otherwise.
	if parentIdForBody == "" {
		req.Header.Set("Referer", c.base()+"/drive")
	} else {
		req.Header.Set("Referer", c.base()+"/drive/folders/"+folderIDForList(parentID))
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnprocessableEntity && strings.Contains(string(bb), "already exists") {
			// Give the server a moment to make the folder visible in list (eventual consistency).
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
			decodedWant := decodeSegment(name)
			nameNorm := strings.TrimSpace(name)
			// Server may return folder with "display" name (e.g. "14" when we created "_14").
			displayWant := strings.TrimLeft(name, "_")
			tryFind := func(entries []FileEntryResponse) string {
				for _, e := range entries {
					if e.Type != "folder" {
						continue
					}
					ename := e.Name.String()
					en := strings.TrimSpace(ename)
					decoded := decodeSegment(ename)
					if ename == name || en == nameNorm || decoded == decodedWant ||
						strings.EqualFold(en, nameNorm) || strings.EqualFold(decoded, decodedWant) ||
						ename == displayWant || decoded == displayWant || strings.EqualFold(decoded, displayWant) {
						// Return "id,hash" so cache can use id for create API and hash for list API.
						if e.Hash != "" {
							return e.ID.String() + "," + e.Hash
						}
						return e.ID.String()
					}
					// Numeric names: match "16280" vs 16280 or "016280" (API may return number or zero-padded).
					if nameNorm != "" && en != "" {
						na, naErr := strconv.ParseInt(strings.TrimLeft(nameNorm, "0"), 10, 64)
						ea, eaErr := strconv.ParseInt(strings.TrimLeft(en, "0"), 10, 64)
						if naErr == nil && eaErr == nil && na == ea {
							if e.Hash != "" {
								return e.ID.String() + "," + e.Hash
							}
							return e.ID.String()
						}
						decNa, decNaErr := strconv.ParseInt(strings.TrimLeft(decodedWant, "0"), 10, 64)
						decEa, decEaErr := strconv.ParseInt(strings.TrimLeft(decoded, "0"), 10, 64)
						if decNaErr == nil && decEaErr == nil && decNa == decEa {
							if e.Hash != "" {
								return e.ID.String() + "," + e.Hash
							}
							return e.ID.String()
						}
					}
				}
				return ""
			}
			// Retry ListAll up to 8 times with backoff (eventual consistency); then fall back to listPageByName.
			var found string
			for attempt := 0; attempt < 8; attempt++ {
				entries, listErr := c.ListAll(ctx, parentID)
				if listErr != nil {
					if attempt < 7 {
						backoff := time.Duration(400*(1<<uint(attempt))) * time.Millisecond
						select {
						case <-ctx.Done():
							return "", ctx.Err()
						case <-time.After(backoff):
						}
					}
					continue
				}
				if id := tryFind(entries); id != "" {
					found = id
					break
				}
				if attempt < 7 {
					backoff := time.Duration(400*(1<<uint(attempt))) * time.Millisecond
					select {
					case <-ctx.Done():
						return "", ctx.Err()
					case <-time.After(backoff):
					}
				}
			}
			if found != "" {
				return found, nil
			}
			for page := 1; page <= 200; page++ {
				byName, _, err := c.listPageByName(ctx, parentID, page, 50)
				if err != nil || len(byName) == 0 {
					break
				}
				if id := tryFind(byName); id != "" {
					return id, nil
				}
				// Stop only on partial page; API may report last_page=1 incorrectly.
				if len(byName) < 50 {
					break
				}
			}
			// Last chance: wait a bit and try ListAll once more (strong eventual consistency).
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(1500 * time.Millisecond):
			}
			if entries, listErr := c.ListAll(ctx, parentID); listErr == nil {
				if id := tryFind(entries); id != "" {
					return id, nil
				}
			}
		}
		return "", fmt.Errorf("create folder failed: %s (%s)", resp.Status, string(bb))
	}
	var out CreateFolderResponse
	if err := json.Unmarshal(bb, &out); err != nil {
		return "", err
	}
	return out.Folder.ID.String(), nil
}

// EnsureFolderPath resolves path (e.g. "a/b/c") to a folder ID, creating missing parents.
// baseFolderID is the root folder ID to start from (e.g. from List("")).
// Concurrent calls for the same parent+segment are serialized and cached so only one
// goroutine creates a given folder; others wait and reuse the result.
// If onFolderCreated is non-nil, it is called for each path segment that was created (e.g. "a", "a/b").
func (c *Client) EnsureFolderPath(ctx context.Context, baseFolderID, path string, onFolderCreated func(createdPath string)) (string, error) {
	if path == "" || path == "." {
		return baseFolderID, nil
	}
	parts := splitPath(path)
	currentID := baseFolderID
	var currentPath string
	for _, name := range parts {
		encoded := encodeSegment(name)
		id, created, err := c.ensureFolder(ctx, currentID, name, encoded)
		if err != nil {
			return "", err
		}
		if currentPath == "" {
			currentPath = name
		} else {
			currentPath = currentPath + "/" + name
		}
		if created && onFolderCreated != nil {
			onFolderCreated(currentPath)
		}
		currentID = id
	}
	return currentID, nil
}

// ensureFolder resolves or creates a single folder segment under parentID.
// Returns (folderID, created bool, error). created is true only when the folder was created by this call.
// Uses per-key locking and caching so concurrent callers for the same (parentID, name)
// don't race and only one goroutine issues the create.
func (c *Client) ensureFolder(ctx context.Context, parentID, originalName, encoded string) (string, bool, error) {
	cacheKey := parentID + "\x00" + encoded

	// Fast path: already resolved.
	if cached, ok := c.folderCache.Load(cacheKey); ok {
		return cached.(string), false, nil
	}

	// Acquire a per-key mutex so only one goroutine resolves this segment.
	muIface, _ := c.folderLocks.LoadOrStore(cacheKey, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring lock.
	if cached, ok := c.folderCache.Load(cacheKey); ok {
		return cached.(string), false, nil
	}

	// List parent and look for existing folder.
	entries, err := c.ListAll(ctx, parentID)
	if err != nil {
		return "", false, err
	}
	for _, e := range entries {
		if e.Type != "folder" {
			continue
		}
		ename := e.Name.String()
		en := strings.TrimSpace(ename)
		decoded := decodeSegment(ename)
		if ename == encoded || en == strings.TrimSpace(encoded) || decoded == originalName ||
			strings.EqualFold(en, encoded) || strings.EqualFold(decoded, originalName) {
			// Store "numericId,hash" so list API gets hash and create API gets integer.
			cacheVal := e.ID.String()
			if e.Hash != "" {
				cacheVal = e.ID.String() + "," + e.Hash
			}
			c.folderCache.Store(cacheKey, cacheVal)
			return cacheVal, false, nil
		}
	}

	// Not found; create it.
	id, err := c.CreateFolder(ctx, encoded, parentID)
	if err != nil {
		return "", false, err
	}
	// Create returns "id" or "id,hash"; we may have only numeric id. Resolve hash so cache has "id,hash" for listing.
	cacheVal := id
	if strings.Index(id, ",") < 0 {
		entries2, _ := c.ListAll(ctx, parentID)
		for _, e := range entries2 {
			if e.Type != "folder" {
				continue
			}
			ename := e.Name.String()
			decoded := decodeSegment(ename)
			if ename == encoded || decoded == originalName || strings.TrimLeft(encoded, "_") == decoded || strings.TrimLeft(encoded, "_") == ename {
				if e.Hash != "" {
					cacheVal = id + "," + e.Hash
				}
				break
			}
		}
	}
	c.folderCache.Store(cacheKey, cacheVal)
	return cacheVal, true, nil
}

// folderFortMinNameLen is FolderFort's minimum folder name length (API returns 422 if shorter).
const folderFortMinNameLen = 3

// encodeSegment returns a name safe for FolderFort (at least folderFortMinNameLen chars).
// Short segments are padded with leading underscores (e.g. "14" -> "_14", "1" -> "__1").
func encodeSegment(s string) string {
	if len(s) >= folderFortMinNameLen {
		return s
	}
	return strings.Repeat("_", folderFortMinNameLen-len(s)) + s
}

// decodeSegment reverses encodeSegment for path segments returned by the API.
func decodeSegment(s string) string {
	if len(s) != folderFortMinNameLen || folderFortMinNameLen == 0 {
		return s
	}
	if s[0] != '_' {
		return s
	}
	if folderFortMinNameLen > 1 && s[1] == '_' {
		return s[2:] // "__x" -> "x"
	}
	return s[1:] // "_xy" -> "xy"
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
	entries, err := c.ListAll(ctx, "")
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.Type == "folder" && (baseName == "" || e.Name.String() == baseName) {
			return e.ID.String(), nil
		}
	}
	if baseName == "" {
		return "", fmt.Errorf("no folder at root; specify a base folder name")
	}
	return c.CreateFolder(ctx, baseName, "")
}

// Presign gets a presigned S3 URL for upload.
// parentID can be cache value "numericId,hash"; API expects integer in body.
func (c *Client) Presign(ctx context.Context, filename, mime string, size int64, extension, parentID string) (*PresignResponse, error) {
	parentIdForBody := folderIDForCreate(parentID)
	body := PresignRequest{
		TotalFileCount: 1,
		Filename:       filename,
		Mime:           mime,
		Disk:           "uploads",
		Size:           size,
		Extension:      extension,
		WorkspaceID:    0,
		ParentID:       parentIdForBody,
		RelativePath:   "",
	}
	raw, _ := json.Marshal(body)
	req, err := c.authReq(ctx, http.MethodPost, "/api/v1/s3/simple/presign", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", c.base()+"/drive/folders/"+folderIDForList(parentID))

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
// parentID can be cache value "numericId,hash"; API expects integer in body.
func (c *Client) CreateEntry(ctx context.Context, parentID, clientMime, clientName, filename string, size int64, extension string) (string, error) {
	parentIdForBody := folderIDForCreate(parentID)
	body := CreateEntryRequest{
		WorkspaceID:      0,
		ParentID:         parentIdForBody,
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
	req.Header.Set("Referer", c.base()+"/drive/folders/"+folderIDForList(parentID))

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

// ListRecursiveProgress is called with delta (files, folders) added at each step during ListRecursive.
// Pass nil to disable. The caller should accumulate deltas for a running total.
type ListRecursiveProgress func(deltaFiles, deltaFolders int)

// ListRecursive returns all files and folders under folderID with paths relative to prefix.
// prefix is the path prefix for the current folder (e.g. "" or "a/b").
// Subfolders are listed concurrently (bounded by ListRecurseConcurrency) to speed up startup.
// When progress is non-nil, it is called with (deltaFiles, deltaFolders) after the initial list and after each subfolder merge.
func (c *Client) ListRecursive(ctx context.Context, folderID, prefix string, progress ListRecursiveProgress) (files map[string]FileEntryResponse, folders map[string]string, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	files = make(map[string]FileEntryResponse)
	folders = make(map[string]string)
	// Limit concurrent folder listings. Important: don't hold the semaphore across recursion,
	// otherwise parent goroutines can deadlock waiting for children to acquire a token.
	entries, err := func() ([]FileEntryResponse, error) {
		select {
		case c.listRecurseSem <- struct{}{}:
			// acquired
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		defer func() { <-c.listRecurseSem }()
		return c.ListAll(ctx, folderID)
	}()
	if err != nil {
		return nil, nil, err
	}
	var subfolders []struct {
		entry    FileEntryResponse
		fullPath string
	}
	for _, e := range entries {
		decodedName := decodeSegment(e.Name.String())
		fullPath := decodedName
		if prefix != "" {
			fullPath = prefix + "/" + decodedName
		}
		if e.Type == "folder" {
			folders[fullPath] = e.ID.String()
			subfolders = append(subfolders, struct {
				entry    FileEntryResponse
				fullPath string
			}{e, fullPath})
		} else {
			files[fullPath] = e
		}
	}
	if progress != nil {
		progress(len(files), len(folders))
	}
	if len(subfolders) == 0 {
		return files, folders, nil
	}
	type subResult struct {
		files   map[string]FileEntryResponse
		folders map[string]string
		err     error
	}
	// No per-folder timeout: large folders (e.g. years_1/all with many pages) need time to finish.
	// Each HTTP request uses the client's timeout (20s), so a single stuck request will fail.
	resultCh := make(chan subResult, len(subfolders))
	for _, sf := range subfolders {
		entry, fullPath := sf.entry, sf.fullPath
		go func() {
			folderID := entry.Hash
			if folderID == "" {
				folderID = entry.ID.String()
			}
			subFiles, subFolders, listErr := c.ListRecursive(ctx, folderID, fullPath, progress)
			if listErr != nil && fullPath != "" {
				listErr = fmt.Errorf("list folder %q: %w", fullPath, listErr)
			}
			resultCh <- subResult{files: subFiles, folders: subFolders, err: listErr}
		}()
	}
	var firstErr error
	for range subfolders {
		r := <-resultCh
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
				cancel()
			}
			continue
		}
		for k, v := range r.files {
			files[k] = v
		}
		for k, v := range r.folders {
			folders[k] = v
		}
		if progress != nil {
			progress(len(r.files), len(r.folders))
		}
	}
	if firstErr != nil {
		return nil, nil, firstErr
	}
	return files, folders, nil
}

// DeleteFileEntry deletes a single file or folder by ID (uses bulk delete API with one ID).
func (c *Client) DeleteFileEntry(ctx context.Context, entryID string) error {
	return c.DeleteFileEntries(ctx, []string{entryID}, true)
}

// DeleteFileEntries deletes multiple files/folders via POST /api/v1/file-entries/delete (same as web UI).
// deleteForever: true = permanent delete; false = move to trash.
func (c *Client) DeleteFileEntries(ctx context.Context, entryIDs []string, deleteForever bool) error {
	if len(entryIDs) == 0 {
		return nil
	}
	body := DeleteFileEntriesRequest{EntryIDs: entryIDs, DeleteForever: deleteForever}
	raw, _ := json.Marshal(body)
	req, err := c.authReq(ctx, http.MethodPost, "/api/v1/file-entries/delete", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed: %s (%s)", resp.Status, string(bb))
	}
	return nil
}

// EmptyTrash permanently deletes all items in trash (POST /api/v1/file-entries/delete with emptyTrash: true).
func (c *Client) EmptyTrash(ctx context.Context) error {
	body := DeleteFileEntriesRequest{EntryIDs: []string{}, EmptyTrash: true}
	raw, _ := json.Marshal(body)
	req, err := c.authReq(ctx, http.MethodPost, "/api/v1/file-entries/delete", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("empty trash failed: %s (%s)", resp.Status, string(bb))
	}
	return nil
}

// SpaceUsage returns the user's storage usage (GET /api/v1/user/space-usage).
// Used and Available are in bytes.
func (c *Client) SpaceUsage(ctx context.Context) (*SpaceUsageResponse, error) {
	req, err := c.authReq(ctx, http.MethodGet, "/api/v1/user/space-usage", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("space-usage failed: %s (%s)", resp.Status, string(bb))
	}
	var out SpaceUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
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
		if s, ok := body.(io.Seeker); ok {
			if _, seekErr := s.Seek(0, io.SeekStart); seekErr != nil {
				return seekErr
			}
		}
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
