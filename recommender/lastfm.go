package recommender

//
// import (
// 	"encoding/json"
// 	"fmt"
// 	"net/http"
// 	"net/url"
// 	"time"
// )
//
// const baseURL = "https://ws.audioscrobbler.com/2.0/"
//
// // Client holds the API key and an HTTP client with a timeout.
// type LastFMClient struct {
// 	apiKey     string
// 	httpClient *http.Client
// }
//
// func NewLastFM(apiKey string) *LastFMClient {
// 	return &LastFMClient{
// 		apiKey: apiKey,
// 		httpClient: &http.Client{
// 			Timeout: 10 * time.Second,
// 		},
// 	}
// }
//
// type LastFMTrack struct {
// 	Name   string `json:"name"`
// 	Artist string
// }
//
// type SimilarTrack struct {
// 	LastFMTrack
// 	Match float64 `json:"match"`
// }
//
// // raw response shapes only used for JSON unmarshalling, not exported
// type similarResponse struct {
// 	SimilarTracks struct {
// 		Tracks []struct {
// 			Name   string `json:"name"`
// 			MBID   string `json:"mbid"`
// 			Match  string `json:"match"`
// 			Artist []struct {
// 				Name string `json:"name"`
// 			} `json:"artist"`
// 		} `json:"track"`
// 	} `json:"similartracks"`
// 	Error   int    `json:"error"`
// 	Message string `json:"message"`
// }
//
// func (c *LastFMClient) GetSimilar(title, artist string, limit int) ([]SimilarTrack, error) {
// 	params := url.Values{}
// 	params.Set("method", "track.getsimilar")
// 	params.Set("artist", artist)
// 	params.Set("track", title)
// 	params.Set("autocorrect", "1") // auto-fix typos in artist/track names
// 	params.Set("api_key", c.apiKey)
// 	params.Set("format", "json")
// 	if limit > 0 {
// 		params.Set("limit", fmt.Sprintf("%d", limit))
// 	}
//
// 	var raw similarResponse
// 	if err := c.get(params, &raw); err != nil {
// 		return nil, err
// 	}
// 	if raw.Error != 0 {
// 		return nil, fmt.Errorf("last.fm error %d: %s", raw.Error, raw.Message)
// 	}
//
// 	results := make([]SimilarTrack, 0, len(raw.SimilarTracks.Tracks))
// 	for _, t := range raw.SimilarTracks.Tracks {
// 		st := SimilarTrack{}
// 		st.Name = t.Name
//
// 		// artist is an array in the response but always has exactly one entry
// 		if len(t.Artist) > 0 {
// 			st.Artist = t.Artist[0].Name
// 		}
//
// 		// parse match score — Last.fm returns it as a string like "0.892731"
// 		fmt.Sscanf(t.Match, "%f", &st.Match)
//
// 		results = append(results, st)
// 	}
//
// 	return results, nil
// }
//
// func (c *LastFMClient) get(params url.Values, dest any) error {
// 	reqURL := baseURL + "?" + params.Encode()
//
// 	resp, err := c.httpClient.Get(reqURL)
// 	if err != nil {
// 		return fmt.Errorf("lastfm: http get failed: %w", err)
// 	}
// 	defer resp.Body.Close()
//
// 	if resp.StatusCode != http.StatusOK {
// 		return fmt.Errorf("lastfm: unexpected status %d", resp.StatusCode)
// 	}
//
// 	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
// 		return fmt.Errorf("lastfm: failed to decode response: %w", err)
// 	}
//
// 	return nil
// }
