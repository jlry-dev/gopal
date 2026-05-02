package recommender

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
)

type Recommender interface {
	GetSimilarTrack(title, author string) string
}

type reccomndr struct {
	logger *slog.Logger
}

func NewReccomender(logger *slog.Logger) Recommender {
	return &reccomndr{
		logger: logger,
	}
}

func (r *reccomndr) GetSimilarTrack(title, author string) string {
	spotifyID, _ := getSpotifyID(fmt.Sprintf("track:%v artist:%v", title, author))

	reccobeats, err := getReccoBeatsID(spotifyID)
	if err != nil {
		r.logger.Error("failed to fetch similar track", "error", err)
	}

	if reccobeats != "" {
		feature, err := getReccoBeatsFeature(reccobeats)
		if err != nil || feature == nil {
			r.logger.Error("failed to fetch similar track", "error", err)
			return ""
		}

		fmt.Printf("ID: %v\n", feature.ID)
		fmt.Printf("Energy: %v\n", feature.Energy)
		fmt.Printf("Valence: %v\n", feature.Valence)

		return ""

	}

	return ""
}

func getAccessToken() (string, error) {
	clientID, ok := os.LookupEnv("SPOTIFY_CLIENT_ID")
	if !ok {
		return "", errors.New("missing SPOTIFY_CLIENT_ID env vars")
	}
	clientSecret, ok := os.LookupEnv("SPOTIFY_CLIENT_SECRET")
	if !ok {
		return "", errors.New("missing SPOTIFY_CLIENT_ID env vars")
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", bytes.NewBufferString(data.Encode()))
	if err != nil {
		return "", err
	}

	auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result map[string]any
	json.Unmarshal(body, &result)

	token := result["access_token"].(string)

	return token, nil
}

func getSpotifyID(title string) (string, error) {
	token, _ := getAccessToken()

	query := url.QueryEscape(title)
	endpoint := fmt.Sprintf("https://api.spotify.com/v1/search?q=%s&type=track&limit=1", query)

	req, _ := http.NewRequest("GET", endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("reccobeats get id:: JSON decode error: %w", err)
	}

	tracks := result["tracks"].(map[string]any)
	items := tracks["items"].([]any)

	if len(items) == 0 {
		return "", fmt.Errorf("no track found")
	}

	first := items[0].(map[string]any)
	return first["id"].(string), nil
}
