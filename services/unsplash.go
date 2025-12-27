package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
)

const (
	UNSPLASH_API_BASE_URL = "https://api.unsplash.com"
)

// UnsplashPhoto represents a photo from Unsplash API
type UnsplashPhoto struct {
	ID             string             `json:"id"`
	CreatedAt      string             `json:"created_at"`
	UpdatedAt      string             `json:"updated_at"`
	Width          int                `json:"width"`
	Height         int                `json:"height"`
	Color          string             `json:"color"`
	BlurHash       string             `json:"blur_hash"`
	Description    *string            `json:"description"`
	AltDescription *string            `json:"alt_description"`
	URLs           UnsplashPhotoURLs  `json:"urls"`
	Links          UnsplashPhotoLinks `json:"links"`
	User           UnsplashUser       `json:"user"`
}

// UnsplashPhotoURLs represents different sizes of photo URLs
type UnsplashPhotoURLs struct {
	Raw     string `json:"raw"`
	Full    string `json:"full"`
	Regular string `json:"regular"`
	Small   string `json:"small"`
	Thumb   string `json:"thumb"`
}

// UnsplashPhotoLinks represents photo links
type UnsplashPhotoLinks struct {
	Self             string `json:"self"`
	HTML             string `json:"html"`
	Download         string `json:"download"`
	DownloadLocation string `json:"download_location"`
}

// UnsplashUser represents a user from Unsplash
type UnsplashUser struct {
	ID              string               `json:"id"`
	Username        string               `json:"username"`
	Name            string               `json:"name"`
	FirstName       string               `json:"first_name"`
	LastName        *string              `json:"last_name"`
	TwitterUsername *string              `json:"twitter_username"`
	PortfolioURL    *string              `json:"portfolio_url"`
	Bio             *string              `json:"bio"`
	Location        *string              `json:"location"`
	Links           UnsplashUserLinks    `json:"links"`
	ProfileImage    UnsplashProfileImage `json:"profile_image"`
}

// UnsplashUserLinks represents user links
type UnsplashUserLinks struct {
	Self      string `json:"self"`
	HTML      string `json:"html"`
	Photos    string `json:"photos"`
	Likes     string `json:"likes"`
	Portfolio string `json:"portfolio"`
	Following string `json:"following"`
	Followers string `json:"followers"`
}

// UnsplashProfileImage represents user profile image URLs
type UnsplashProfileImage struct {
	Small  string `json:"small"`
	Medium string `json:"medium"`
	Large  string `json:"large"`
}

// UnsplashSearchResponse represents the response from search photos API
type UnsplashSearchResponse struct {
	Total      int             `json:"total"`
	TotalPages int             `json:"total_pages"`
	Results    []UnsplashPhoto `json:"results"`
}

// UnsplashService allows making requests to the Unsplash API
type UnsplashService struct {
	logger    *slog.Logger
	accessKey string
	secretKey string
}

// NewUnsplashService creates a new Unsplash handler
func NewUnsplashService(accessKey, secretKey string) *UnsplashService {
	logger := slog.With("service", "UnsplashService")

	return &UnsplashService{
		logger:    logger,
		accessKey: accessKey,
		secretKey: secretKey,
	}
}

// SearchPhotos searches Unsplash for photos matching the query
func (s *UnsplashService) SearchPhotos(ctx context.Context, query string, page, perPage int, orientation, orderBy string) (*UnsplashSearchResponse, error) {
	apiURL, err := url.Parse(fmt.Sprintf("%s/search/photos", UNSPLASH_API_BASE_URL))
	if err != nil {
		return nil, err
	}

	params := apiURL.Query()
	params.Set("query", query)
	params.Set("page", strconv.Itoa(page))
	params.Set("per_page", strconv.Itoa(perPage))
	if orderBy != "" {
		params.Set("order_by", orderBy)
	}
	if orientation != "" {
		params.Set("orientation", orientation)
	}
	apiURL.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", apiURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Client-ID "+s.accessKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error calling Unsplash API: %v", string(body))
	}

	var searchResp UnsplashSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}
	return &searchResp, nil
}

// GetPhoto retrieves a single photo by its ID from Unsplash
func (s *UnsplashService) GetPhoto(photoID string) (*UnsplashPhoto, error) {
	apiURL := fmt.Sprintf("%s/photos/%s", UNSPLASH_API_BASE_URL, url.PathEscape(photoID))

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Client-ID "+s.accessKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error calling Unsplash API: %v", string(body))
	}

	var photo UnsplashPhoto
	if err := json.NewDecoder(resp.Body).Decode(&photo); err != nil {
		return nil, err
	}
	return &photo, nil
}
