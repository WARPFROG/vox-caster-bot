package wiki

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchPageImage_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("titles") != "Test_Page" {
			t.Errorf("titles = %q, want %q", r.URL.Query().Get("titles"), "Test_Page")
		}
		w.Write([]byte(`{
			"query": {
				"pages": {
					"123": {
						"pageid": 123,
						"title": "Test Page",
						"thumbnail": {
							"source": "https://wiki.example.com/images/thumb/test.jpg",
							"width": 500,
							"height": 800
						},
						"pageimage": "test.jpg"
					}
				}
			}
		}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, false)
	img, err := c.FetchPageImage(context.Background(), "Test_Page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if img != "https://wiki.example.com/images/thumb/test.jpg" {
		t.Errorf("image = %q, want thumbnail URL", img)
	}
}

func TestFetchPageImage_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"query": {
				"pages": {
					"-1": {
						"ns": 0,
						"title": "Nonexistent",
						"missing": ""
					}
				}
			}
		}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, false)
	img, err := c.FetchPageImage(context.Background(), "Nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if img != "" {
		t.Errorf("expected empty image URL for missing page, got %q", img)
	}
}

func TestFetchPageImage_NoImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"query": {
				"pages": {
					"456": {
						"pageid": 456,
						"title": "No Image Page"
					}
				}
			}
		}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, false)
	img, err := c.FetchPageImage(context.Background(), "No_Image_Page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if img != "" {
		t.Errorf("expected empty image URL, got %q", img)
	}
}

func TestPageTitleFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{
			"https://wiki.example.com/index.php?title=Some_Page&diff=123&oldid=456",
			"Some_Page",
		},
		{
			"https://wiki.example.com/index.php?title=%D0%A2%D0%B5%D1%81%D1%82&diff=1",
			"\u0422\u0435\u0441\u0442", // "Тест"
		},
		{
			"https://wiki.example.com/index.php",
			"",
		},
		{
			"not a url %%%",
			"",
		},
	}
	for _, tt := range tests {
		got := PageTitleFromURL(tt.url)
		if got != tt.want {
			t.Errorf("PageTitleFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestDirectPageURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{
			"https://wiki.example.com/index.php?title=Some_Page&diff=123&oldid=456",
			"https://wiki.example.com/index.php?title=Some_Page",
		},
		{
			"https://wiki.example.com/index.php?title=%D0%A2%D0%B5%D1%81%D1%82&diff=1&oldid=2",
			"https://wiki.example.com/index.php?title=%D0%A2%D0%B5%D1%81%D1%82",
		},
		{
			"https://wiki.example.com/index.php",
			"https://wiki.example.com/index.php",
		},
	}
	for _, tt := range tests {
		got := DirectPageURL(tt.in)
		if got != tt.want {
			t.Errorf("DirectPageURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
