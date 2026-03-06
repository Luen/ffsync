package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginAndList(t *testing.T) {
	xsrf := "test-xsrf-token"
	var loginCalled bool
	var listCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/login" && r.Method == http.MethodGet:
			http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: xsrf, Path: "/"})
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/auth/login" && r.Method == http.MethodPost:
			loginCalled = true
			if r.Header.Get("X-XSRF-TOKEN") != xsrf {
				t.Error("missing X-XSRF-TOKEN header")
			}
			var body LoginRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Email != "u@e.com" || body.Password != "secret" {
				t.Errorf("unexpected body: %+v", body)
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/drive/file-entries" && r.Method == http.MethodGet:
			listCalled = true
			if r.Header.Get("X-XSRF-TOKEN") != xsrf {
				t.Error("missing X-XSRF-TOKEN on list")
			}
			if r.URL.Query().Get("folderId") != "f1" {
				t.Errorf("unexpected folderId: %s", r.URL.Query().Get("folderId"))
			}
			json.NewEncoder(w).Encode(FileEntriesResponse{
				Data: []FileEntryResponse{
					{ID: "e1", Name: "file1.txt", Type: "file", Size: 10},
					{ID: "e2", Name: "dir1", Type: "folder"},
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cl, err := New(srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := cl.Login(ctx, "u@e.com", "secret"); err != nil {
		t.Fatal(err)
	}
	if !loginCalled {
		t.Error("login was not called")
	}
	entries, err := cl.List(ctx, "f1")
	if err != nil {
		t.Fatal(err)
	}
	if !listCalled {
		t.Error("list was not called")
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name.String() != "file1.txt" || entries[1].Name.String() != "dir1" {
		t.Errorf("unexpected entries: %+v", entries)
	}
}

func TestCreateFolder(t *testing.T) {
	xsrf := "xsrf2"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" && r.Method == http.MethodGet {
			http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: xsrf, Path: "/"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/auth/login" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/v1/folders" && r.Method == http.MethodPost {
			var body CreateFolderRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Name != "mydir" {
				t.Errorf("unexpected name: %s", body.Name)
			}
			json.NewEncoder(w).Encode(CreateFolderResponse{
				Folder: struct {
					ID FlexID `json:"id"`
				}{ID: "new-folder-id"},
			})
			return
		}
		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cl, _ := New(srv.URL, "")
	ctx := context.Background()
	_ = cl.Login(ctx, "u@e.com", "secret")
	id, err := cl.CreateFolder(ctx, "mydir", "parent-id")
	if err != nil {
		t.Fatal(err)
	}
	if id != "new-folder-id" {
		t.Errorf("expected new-folder-id, got %s", id)
	}
}

func TestPresignAndUpload(t *testing.T) {
	xsrf := "xsrf3"
	var presignBody PresignRequest
	var s3Put bool
	s3Srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			s3Put = true
			if r.Header.Get("Content-Type") != "text/plain" {
				t.Errorf("Content-Type: %s", r.Header.Get("Content-Type"))
			}
			if r.Header.Get("x-amz-acl") != "private" {
				t.Errorf("x-amz-acl: %s", r.Header.Get("x-amz-acl"))
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != "hello" {
				t.Errorf("body: %q", string(body))
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer s3Srv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" && r.Method == http.MethodGet {
			http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: xsrf, Path: "/"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/auth/login" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/v1/s3/simple/presign" && r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&presignBody)
			json.NewEncoder(w).Encode(PresignResponse{
				URL: s3Srv.URL + "/upload",
				Key: "uploads/abc123.txt",
				ACL: "private",
			})
			return
		}
		if r.URL.Path == "/api/v1/s3/entries" && r.Method == http.MethodPost {
			var body CreateEntryRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.ClientName != "test.txt" || body.Size != 5 {
				t.Errorf("entry: %+v", body)
			}
			json.NewEncoder(w).Encode(CreateEntryResponse{
				FileEntry: struct {
					ID FlexID `json:"id"`
				}{ID: "entry-1"},
			})
			return
		}
		t.Errorf("unexpected api: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer apiSrv.Close()

	cl, _ := New(apiSrv.URL, "")
	ctx := context.Background()
	_ = cl.Login(ctx, "u@e.com", "secret")
	entryID, err := cl.UploadFile(ctx, "parent-id", "test.txt", "text/plain", "txt", 5, strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if entryID != "entry-1" {
		t.Errorf("entry ID: %s", entryID)
	}
	if !s3Put {
		t.Error("S3 PUT was not called")
	}
	if presignBody.Filename != "test.txt" || presignBody.Size != 5 {
		t.Errorf("presign body: %+v", presignBody)
	}
}
