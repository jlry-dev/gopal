package recommender

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

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

	return &reccomndr{
		logger:   logger,
		selector: DefaultSelector(rand.New(rand.NewSource(time.Now().UnixNano()))),
		lastFM:   lastFM,
	}
}

func (r *reccomndr) GetSimilarTrack(title, author string) string {
	sessionSpotifyID, err := getSpotifyID(fmt.Sprintf("track:%v artist:%v", title, author))
	if err != nil {
		r.logger.Error("failed to resolve spotify id", "error", err)
		return ""
	}

	sessionTrackRBID, err := getReccoBeatsID(sessionSpotifyID)
	if err != nil {
		r.logger.Error("failed to fetch similar track", "error", err)
	}

	// get candidates
	candidates, err := r.lastFM.GetSimilar(title, author, 10)
	if err != nil {
		r.logger.Error("failed to fetch similar track", "error", err)
		return ""
	}

	ids := []string{}
	idMu := sync.Mutex{}
	wg := sync.WaitGroup{}
	for _, t := range candidates {
		wg.Go(func() {
			id, err := getSpotifyID(fmt.Sprintf("track:%v artist:%v", t.Name, t.Artist))
			if err != nil {
				r.logger.Error("failed to resolve spotify id", "error", err)
				return
			}

			idMu.Lock()
			defer idMu.Unlock()
			ids = append(ids, id)
		})
	}

	wg.Wait()

	ri, err := getMultipleTracksInfo(strings.Join(ids, ","))
	if err != nil {
		r.logger.Error("failed to fetch infods", "error", err)
		return ""
	}

	titleMap := map[string]titleMapStruct{}

	for _, t := range ri {
		s := titleMapStruct{}
		s.title = t.TrackTitle
		s.artist = t.Artists[0].Name
		s.concat = fmt.Sprintf("%v - %v", t.TrackTitle, t.Artists[0].Name)
		titleMap[t.ID] = s

	}

	ids = append(ids, sessionSpotifyID)

	features, err := getReccoBeatsMultiFeatures(strings.Join(ids, ","))
	if err != nil {
		r.logger.Error("failed to fetch features", "error", err)
		return ""
	}

	var seed Track
	pool := make([]Track, 0, len(features))

	for _, t := range features {
		track := Track{
			ID:          t.ID,
			TrackTitle:  titleMap[t.ID].title,
			TrackArtist: titleMap[t.ID].artist,
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
	token, err := getAccessToken()
	if err != nil {
		log.Fatal(err)
	}

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
