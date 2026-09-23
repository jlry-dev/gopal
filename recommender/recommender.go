package recommender

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

var defaultHTTPClient = &http.Client{Timeout: 10 * time.Second}

type Recommender interface {
	GetSimilarTrack(title, author string) string
}

type reccomndr struct {
	logger   *slog.Logger
	selector *Selector
	lastFM   *LastFMClient
}

type titleMapStruct struct {
	title  string
	artist string
	concat string
}

func NewReccomender(logger *slog.Logger) Recommender {
	lastFM := NewLastFM()
	if lastFM.apiKey == "" {
		logger.Warn("LAST_FM_KEY not set, similar-track recommendations will be unavailable")
	}

	return &reccomndr{
		logger:   logger,
		selector: DefaultSelector(rand.New(rand.NewSource(time.Now().UnixNano()))),
		lastFM:   lastFM,
	}
}

func (r *reccomndr) GetSimilarTrack(title, author string) string {
	sessionSpotifyID, err := getSpotifyID(title)
	if err != nil {
		r.logger.Error("failed to resolve spotify id", "error", err)
		return ""
	}

	sessionTrackRBID, err := getReccoBeatsID(sessionSpotifyID)
	if err != nil {
		r.logger.Error("failed to resolve reccobeats id", "error", err)
		return ""
	}
	if sessionTrackRBID == "" {
		r.logger.Error("seed track not found on reccobeats", "spotify_id", sessionSpotifyID)
		return ""
	}

	// get candidates
	candidates, err := r.lastFM.GetSimilar(title, author, 10)
	if err != nil {
		r.logger.Error("failed to fetch similar tracks from lastfm", "error", err)
		return ""
	}

	ids := make([]string, 0, len(candidates)+1)
	idMu := sync.Mutex{}
	wg := sync.WaitGroup{}
	for _, t := range candidates {
		wg.Go(func() {
			spotifyID, err := getSpotifyID(fmt.Sprintf("track:%v artist:%v", t.Name, t.Artist))
			if err != nil {
				r.logger.Warn("failed to resolve candidate spotify id", "track", t.Name, "artist", t.Artist, "error", err)
				return
			}

			rbID, err := getReccoBeatsID(spotifyID)
			if err != nil {
				r.logger.Warn("failed to resolve candidate reccobeats id", "track", t.Name, "artist", t.Artist, "error", err)
				return
			}
			if rbID == "" {
				return
			}

			idMu.Lock()
			ids = append(ids, rbID)
			idMu.Unlock()
		})
	}

	wg.Wait()

	ids = append(ids, sessionTrackRBID)

	ri, err := getMultipleTracksInfo(strings.Join(ids, ","))
	if err != nil {
		r.logger.Error("failed to fetch track info", "error", err)
		return ""
	}

	titleMap := map[string]titleMapStruct{}

	for _, t := range ri {
		if len(t.Artists) == 0 {
			continue
		}
		titleMap[t.ID] = titleMapStruct{
			title:  t.TrackTitle,
			artist: t.Artists[0].Name,
			concat: fmt.Sprintf("%v - %v", t.TrackTitle, t.Artists[0].Name),
		}
	}

	features, err := getReccoBeatsMultiFeatures(strings.Join(ids, ","))
	if err != nil {
		r.logger.Error("failed to fetch audio features", "error", err)
		return ""
	}

	var seed Track
	pool := make([]Track, 0, len(features))

	for _, t := range features {
		meta, ok := titleMap[t.ID]
		if !ok {
			continue
		}

		track := Track{
			ID:          t.ID,
			TrackTitle:  meta.title,
			TrackArtist: meta.artist,
			Features: FeatureVector{
				t.Valence, t.Energy, t.Accousticness, t.Danceability,
				t.Instrumentalness, t.Liveness, t.Speechiness,
				clamp01(t.Tempo / 250),
				clamp01((t.Loudness + 60) / 60),
			},
		}

		if t.ID == sessionTrackRBID {
			seed = track
			continue
		}
		pool = append(pool, track)
	}

	if seed.ID == "" {
		r.logger.Error("seed track missing from feature response", "id", sessionTrackRBID)
		return ""
	}
	if len(pool) == 0 {
		r.logger.Error("no candidates left after removing seed")
		return ""
	}

	state := NewSessionState(len(seed.Features))
	state.Update(seed.Features, 0.25)

	ranked := RankTracks(pool, state, nil, DefaultScoreParams())
	chosen, err := r.selector.Select(ranked, state)
	if err != nil {
		r.logger.Error("selection failed", "error", err)
		return ""
	}

	return fmt.Sprintf("%v - %v", chosen.TrackTitle, chosen.TrackArtist)
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

var (
	tokenMu      sync.Mutex
	cachedToken  string
	tokenExpires time.Time
)

func getAccessToken() (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if cachedToken != "" && time.Now().Before(tokenExpires.Add(-60*time.Second)) {
		return cachedToken, nil
	}

	clientID, ok := os.LookupEnv("SPOTIFY_CLIENT_ID")
	if !ok {
		return "", errors.New("missing SPOTIFY_CLIENT_ID env variable")
	}
	clientSecret, ok := os.LookupEnv("SPOTIFY_CLIENT_SECRET")
	if !ok {
		return "", errors.New("missing SPOTIFY_CLIENT_SECRET env variable")
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

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("spotify token request failed with status %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("spotify token JSON decode error: %w", err)
	}

	if result.AccessToken == "" {
		return "", errors.New("spotify token response missing access_token")
	}

	expiresIn := time.Duration(result.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = time.Hour
	}

	cachedToken = result.AccessToken
	tokenExpires = time.Now().Add(expiresIn)

	return cachedToken, nil
}

func getSpotifyID(title string) (string, error) {
	token, err := getAccessToken()
	if err != nil {
		return "", err
	}

	query := url.QueryEscape(title)
	endpoint := fmt.Sprintf("https://api.spotify.com/v1/search?q=%s&type=track&limit=1", query)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("spotify search failed with status %d", resp.StatusCode)
	}

	var result struct {
		Tracks struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("spotify search JSON decode error: %w", err)
	}

	if len(result.Tracks.Items) == 0 {
		return "", fmt.Errorf("no track found for %q", title)
	}

	return result.Tracks.Items[0].ID, nil
}
