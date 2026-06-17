package recommender

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type Track struct {
	ID         string `json:"id"`
	TrackTitle string `json:"trackTitle"`
}

type ReccoBeatsResponse struct {
	Content []Track `json:"content"`
}

type AudioFeatures struct {
	ID      string  `json:"id"`
	Energy  float64 `json:"energy"`
	Valence float64 `json:"valence"`
}

type MultipleAudioFeatures struct {
	Content []AudioFeatures `json:"content"`
}

type SimilarTracks struct {
	ID     string `json:"id"`
	Title  string `json:"trackTitle"`
	Artist []struct {
		Name string `json:"name"`
	} `json:"artists"`
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

func getReccoBeatsFeature(id string) (*AudioFeatures, error) {
	apiURL := fmt.Sprintf("https://api.reccobeats.com/v1/track/%v/audio-features", url.QueryEscape(id))

	res, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("reccobeats get feature: request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reccobeats get feature: unexpected status %d", res.StatusCode)
	}

	var data AudioFeatures
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("reccobeats get feature: JSON decode error: %w", err)
	}

	if (data != AudioFeatures{}) {
		return &data, nil
	}

	return nil, fmt.Errorf("reccobeats get feature: no features found for ID %s", id)
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

func getSimilar(id string, limit int) ([]SimilarTracks, error) {
	baseURL := "https://api.reccobeats.com/v1/track/recommendation"
	params := url.Values{}
	params.Set("size", strconv.Itoa(limit))
	params.Set("seeds", id)

	res, err := http.Get(baseURL + "?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("reccobeats get similar: request failed: %w", err)
	}
	defer res.Body.Close()

	var data SimilarResponse
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("reccobeats get similar: JSON decode error: %w", err)
	}

	if len(data.Content) == 0 {
		return nil, fmt.Errorf("reccobeats get similar: no similar tracks found for ID %s", id)
	}

	return data.Content, nil
}
