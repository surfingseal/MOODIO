package service

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"

	"github.com/surfingseal/MOODIO/internal/model"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

// 전역 비디오 ID 캐시 (동일한 곡에 대해 Search.List 100 units 할당량 소모 방지)
var videoCache sync.Map

var youtubeVideoIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)

func isValidVideoID(id string) bool {
	id = strings.TrimSpace(id)
	return youtubeVideoIDRegex.MatchString(id)
}

// PlaylistService YouTube 플레이리스트 작업 인터페이스
type PlaylistService interface {
	CreatePlaylist(ctx context.Context, title, description, privacyStatus string) (string, error)
	SearchVideoID(ctx context.Context, query string) (string, error)
	AddTrack(ctx context.Context, playlistID, videoID string) error
	GeneratePlaylistWithTracks(ctx context.Context, req model.PlaylistRequest) (string, error)
}

// YouTubeService YouTube API 연동 서비스 구현체
type YouTubeService struct {
	api *youtube.Service
}

// NewYouTubeService OAuth 토큰을 기반으로 YouTube API 클라이언트를 초기화합니다.
func NewYouTubeService(ctx context.Context, oauthConfig *oauth2.Config, token *oauth2.Token) (*YouTubeService, error) {
	tokenSource := oauthConfig.TokenSource(ctx, token)
	service, err := youtube.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, fmt.Errorf("YouTube API 클라이언트 생성 실패: %w", err)
	}

	return &YouTubeService{api: service}, nil
}

// CreatePlaylist 사용자의 계정에 새 플레이리스트를 생성하고 Playlist ID를 반환합니다.
func (s *YouTubeService) CreatePlaylist(ctx context.Context, title, description, privacyStatus string) (string, error) {
	if privacyStatus == "" {
		privacyStatus = "public"
	}

	newPlaylist := &youtube.Playlist{
		Snippet: &youtube.PlaylistSnippet{
			Title:       title,
			Description: description,
		},
		Status: &youtube.PlaylistStatus{
			PrivacyStatus: privacyStatus,
		},
	}

	created, err := s.api.Playlists.Insert([]string{"snippet", "status"}, newPlaylist).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("플레이리스트 생성 요청 실패: %w", err)
	}

	return created.Id, nil
}

// SearchVideoID 검색어로 가장 연관도 높은 첫 번째 동영상 ID를 검색합니다. (Quota 100 소모)
func (s *YouTubeService) SearchVideoID(ctx context.Context, query string) (string, error) {
	call := s.api.Search.List([]string{"id"}).
		Q(query).
		Type("video").
		MaxResults(1).
		Context(ctx)

	resp, err := call.Do()
	if err != nil {
		return "", fmt.Errorf("검색 쿼리 실행 실패 (%s): %w", query, err)
	}

	if len(resp.Items) == 0 {
		return "", fmt.Errorf("검색 결과 없음 (%s)", query)
	}

	return resp.Items[0].Id.VideoId, nil
}

// AddTrack 플레이리스트에 동영상을 추가합니다.
func (s *YouTubeService) AddTrack(ctx context.Context, playlistID, videoID string) error {
	playlistItem := &youtube.PlaylistItem{
		Snippet: &youtube.PlaylistItemSnippet{
			PlaylistId: playlistID,
			ResourceId: &youtube.ResourceId{
				Kind:    "youtube#video",
				VideoId: videoID,
			},
		},
	}

	_, err := s.api.PlaylistItems.Insert([]string{"snippet"}, playlistItem).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("플레이리스트 아이템 등록 실패 (videoID: %s): %w", videoID, err)
	}

	return nil
}

// GeneratePlaylistWithTracks 플레이리스트 생성 및 트랙 등록을 수행합니다.
// Gemini가 제공한 VideoID 또는 캐시를 우선 활용하여 YouTube Search API 할당량을 대폭 절약합니다.
func (s *YouTubeService) GeneratePlaylistWithTracks(ctx context.Context, req model.PlaylistRequest) (string, error) {
	playlistID, err := s.CreatePlaylist(ctx, req.Title, req.Description, req.PrivacyStatus)
	if err != nil {
		return "", err
	}

	log.Printf("📁 플레이리스트 생성 완료 (ID: %s, 제목: %s)", playlistID, req.Title)

	for i, track := range req.Tracks {
		var videoID string
		cacheKey := strings.ToLower(strings.TrimSpace(track.Artist + " - " + track.Title))

		// 1. Gemini가 제공한 유효한 VideoID가 있는 경우 (Search API 완전 생략 -> Quota 0 소모)
		if isValidVideoID(track.VideoID) {
			videoID = track.VideoID
			videoCache.Store(cacheKey, videoID)
			log.Printf("⚡ [%d/%d] Gemini 추출 VideoID 직접 활용 (검색 API 생략): %s (ID: %s)", i+1, len(req.Tracks), track, videoID)
		} else if cached, ok := videoCache.Load(cacheKey); ok {
			// 2. 내부 캐시에 있는 경우 (Search API 생략 -> Quota 0 소모)
			videoID = cached.(string)
			log.Printf("📦 [%d/%d] 캐시 히트로 등록 (검색 API 생략): %s (ID: %s)", i+1, len(req.Tracks), track, videoID)
		} else {
			// 3. Fallback: Search.List API로 검색 수행 (100 units 소모)
			query := track.SearchQuery()
			log.Printf("🔍 [%d/%d] YouTube Search API 호출: %s", i+1, len(req.Tracks), query)
			foundID, err := s.SearchVideoID(ctx, query)
			if err != nil {
				log.Printf("[%d/%d] 검색 실패 (%s): %v", i+1, len(req.Tracks), track, err)
				continue
			}
			videoID = foundID
			videoCache.Store(cacheKey, videoID)
		}

		if err := s.AddTrack(ctx, playlistID, videoID); err != nil {
			log.Printf("[%d/%d] 등록 실패 (%s, ID: %s): %v", i+1, len(req.Tracks), track, videoID, err)
			continue
		}

		log.Printf("✅ [%d/%d] 등록 완료: %s (VideoID: %s)", i+1, len(req.Tracks), track, videoID)
	}

	youtubeMusicURL := fmt.Sprintf("https://music.youtube.com/playlist?list=%s", playlistID)
	return youtubeMusicURL, nil
}
