package recommender

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Track pairs a stable identifier and artist with its feature vector.
// The ID is what candidate fusion dedupes on; ArtistID is what the
// diversity cap enforces against.
type Track struct {
	ID          string `json:"id"`
	TrackTitle  string `json:"trackTitle"`
	TrackArtist string `json:"artist"`
	Features    FeatureVector
}

type Artist struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

type TrackInfo struct {
	ID                 string   `json:"id"`
	TrackTitle         string   `json:"trackTitle"`
	Artists            []Artist `json:"artists"`
	DurationMs         int      `json:"durationMs"`
	Isrc               string   `json:"isrc,omitempty"`
	Ean                string   `json:"ean,omitempty"`
	Upc                string   `json:"upc,omitempty"`
	Href               string   `json:"href"`
	AvailableCountries string   `json:"availableCountries,omitempty"`
}

type MultipleTrackInfo struct {
	Content []TrackInfo `json:"content"`
}

type ReccoBeatsResponse struct {
	Content []Track `json:"content"`
}

type AudioFeatures struct {
	ID               string  `json:"id"`
	Energy           float64 `json:"energy"`
	Valence          float64 `json:"valence"`
	Danceability     float64 `json:"danceability"`
	Accousticness    float64 `json:"acousticness"`
	Instrumentalness float64 `json:"instrumentalness"`
	Liveness         float64 `json:"liveness"`
	Speechiness      float64 `json:"speechiness"`
	Tempo            float64 `json:"tempo"`
	Loudness         float64 `json:"loudness"`
}

type MultipleAudioFeatures struct {
	Content []AudioFeatures `json:"content"`
}

type SimilarTracks struct {
	ID      string   `json:"id"`
	Title   string   `json:"trackTitle"`
	Artists []Artist `json:"artists"`
}

type SimilarResponse struct {
	Content []SimilarTracks `json:"content"`
}

func getReccoBeatsID(trackID string) (string, error) {
	apiURL := fmt.Sprintf("https://api.reccobeats.com/v1/track?ids=%v", url.QueryEscape(trackID))

	res, err := http.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("reccobeats get id: request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("reccobeats get id: unexpected status code %d", res.StatusCode)
	}

	var data ReccoBeatsResponse
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("reccobeats get id:: JSON decode error: %w", err)
	}

	if len(data.Content) == 0 {
		return "", nil
	}

	return data.Content[0].ID, nil
}

func getMultipleTracksInfo(ids string) ([]TrackInfo, error) {
	baseURL := "https://api.reccobeats.com/v1/track"
	params := url.Values{}
	params.Set("ids", ids)

	res, err := http.Get(baseURL + "?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("reccobeats get multiple track info: request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reccobeats get multiple track info: unexpected status %d", res.StatusCode)
	}

	var data MultipleTrackInfo
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("reccobeats get multiple track info: JSON decode error: %w", err)
	}

	if len(data.Content) >= 1 {
		return data.Content, nil
	}

	return nil, fmt.Errorf("reccobeats get multiple track info: no features found")
}

func getReccoBeatsMultiFeatures(ids string) ([]AudioFeatures, error) {
	baseURL := "https://api.reccobeats.com/v1/audio-features"
	params := url.Values{}
	params.Set("ids", ids)

	res, err := http.Get(baseURL + "?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("reccobeats get multiple features: request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reccobeats get multiple features: unexpected status %d", res.StatusCode)
	}

	var data MultipleAudioFeatures
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("reccobeats get multiple features: JSON decode error: %w", err)
	}

	if len(data.Content) >= 1 {
		return data.Content, nil
	}

	return nil, fmt.Errorf("reccobeats get multiple features: no features found")
}
