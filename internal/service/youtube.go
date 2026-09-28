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
// 곡 검색을 Goroutine으로 병렬 처리하여 대기 시간을 대폭 단축하고, 등록 시 원래 곡 순서를 보장합니다.
func (s *YouTubeService) GeneratePlaylistWithTracks(ctx context.Context, req model.PlaylistRequest) (string, error) {
	playlistID, err := s.CreatePlaylist(ctx, req.Title, req.Description, req.PrivacyStatus)
	if err != nil {
		return "", err
	}

	log.Printf("📁 플레이리스트 생성 완료 (ID: %s, 제목: %s)", playlistID, req.Title)

	// 1단계: 병렬 비디오 ID 확인 및 검색 (최대 동시 5개 실행)
	totalTracks := len(req.Tracks)
	videoIDs := make([]string, totalTracks)
	sem := make(chan struct{}, 5) // 동시 요청 제한 세마포어
	var wg sync.WaitGroup

	for i, track := range req.Tracks {
		wg.Add(1)
		go func(idx int, t model.Track) {
			defer wg.Done()

			// Gemini가 유효한 VideoID를 이미 제공한 경우 검색 건너뜀
			if isValidVideoID(t.VideoID) {
				videoIDs[idx] = t.VideoID
				log.Printf("⚡ [%d/%d] Gemini 추출 VideoID 직접 활용: %s (ID: %s)", idx+1, totalTracks, t, t.VideoID)
				return
			}

			// 검색이 필요한 경우 세마포어 슬롯 확보 후 병렬 검색 수행
			sem <- struct{}{}
			defer func() { <-sem }()

			query := t.SearchQuery()
			log.Printf("🔍 [%d/%d] 병렬 검색 실행: %s", idx+1, totalTracks, query)
			foundID, err := s.SearchVideoID(ctx, query)
			if err != nil {
				log.Printf("⚠️ [%d/%d] 검색 실패 (%s): %v", idx+1, totalTracks, t, err)
				return
			}
			videoIDs[idx] = foundID
		}(i, track)
	}

	// 모든 병렬 검색 작업 완료 대기
	wg.Wait()

	// 2단계: 원래 순서대로 플레이리스트에 등록 (순서 보장)
	for i, track := range req.Tracks {
		videoID := videoIDs[i]
		if videoID == "" {
			log.Printf("⚠️ [%d/%d] 비디오 ID 누락으로 등록 건너뜀: %s", i+1, totalTracks, track)
			continue
		}

		err := s.AddTrack(ctx, playlistID, videoID)
		if err != nil {
			log.Printf("⚠️ [%d/%d] 1차 등록 실패 (ID: %s), 검색 Fallback 시도: %s", i+1, totalTracks, videoID, track)
			// 잘못된 비디오 ID였을 경우 즉시 검색 API로 재시도
			foundID, searchErr := s.SearchVideoID(ctx, track.SearchQuery())
			if searchErr == nil && foundID != "" {
				if retryErr := s.AddTrack(ctx, playlistID, foundID); retryErr == nil {
					log.Printf("✅ [%d/%d] Fallback 등록 성공: %s (ID: %s)", i+1, totalTracks, track, foundID)
					continue
				}
			}
			log.Printf("❌ [%d/%d] 최종 등록 실패: %s (%v)", i+1, totalTracks, track, err)
			continue
		}

		log.Printf("✅ [%d/%d] 등록 완료: %s (VideoID: %s)", i+1, totalTracks, track, videoID)
	}

	youtubeMusicURL := fmt.Sprintf("https://music.youtube.com/playlist?list=%s", playlistID)
	return youtubeMusicURL, nil
}
