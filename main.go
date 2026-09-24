package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

type Track struct {
	Artist string
	Title  string
}

const oauthStateString = "random-music-state"

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  getEnv("REDIRECT_URL", "http://localhost:8080/auth/google/callback"),
		Scopes: []string{
			"https://www.googleapis.com/auth/youtube",
			"https://www.googleapis.com/auth/userinfo.email",
		},
		Endpoint: google.Endpoint,
	}
}

func main() {
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/auth/google/login", handleGoogleLogin)
	http.HandleFunc("/auth/google/callback", handleGoogleCallback)

	port := getEnv("PORT", "8080")
	fmt.Printf("🚀 서버 실행 중: 포트 %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// 기본 홈 화면 (테스트용 간단 안내 및 시작 링크)
func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	html := `
	<!DOCTYPE html>
	<html>
	<head><meta charset="UTF-8"><title>YouTube Music Playlist Generator</title></head>
	<body style="font-family: sans-serif; text-align: center; padding-top: 50px;">
		<h2>🎵 풍경 기반 유튜브 뮤직 플레이리스트 생성</h2>
		<p>아래 버튼을 눌러 Google 계정으로 로그인하면 내 유튜브 보관함에 3곡 플레이리스트가 생성됩니다.</p>
		<a href="/auth/google/login" style="display:inline-block; padding:12px 24px; background:#FF0000; color:#fff; text-decoration:none; border-radius:5px; font-weight:bold;">
			YouTube 계정 연동 및 생성하기
		</a>
	</body>
	</html>
	`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// 1. 구글 인증 화면으로 리다이렉트
func handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	oauthConfig := getOAuthConfig()
	url := oauthConfig.AuthCodeURL(oauthStateString, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// 2. 인증 콜백 및 플레이리스트 자동 등록
func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("state") != oauthStateString {
		http.Error(w, "State 값이 일치하지 않습니다.", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	ctx := context.Background()
	oauthConfig := getOAuthConfig()

	// 코드를 Access Token으로 교환
	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		http.Error(w, fmt.Sprintf("토큰 발급 실패: %v", err), http.StatusInternalServerError)
		return
	}

	// YouTube 서비스 클라이언트 생성
	youtubeService, err := youtube.NewService(ctx, option.WithTokenSource(oauthConfig.TokenSource(ctx, token)))
	if err != nil {
		http.Error(w, fmt.Sprintf("YouTube API 클라이언트 생성 실패: %v", err), http.StatusInternalServerError)
		return
	}

	// 플레이리스트 신규 생성 (사용자 유튜브 계정 보관함)
	newPlaylist := &youtube.Playlist{
		Snippet: &youtube.PlaylistSnippet{
			Title:       "비 내리는 늦은 오후의 산책 🌧️",
			Description: "사진 분위기 맞춤 자동 선곡 플레이리스트 (3곡)",
		},
		Status: &youtube.PlaylistStatus{
			PrivacyStatus: "public",
		},
	}

	createdPlaylist, err := youtubeService.Playlists.Insert([]string{"snippet", "status"}, newPlaylist).Do()
	if err != nil {
		http.Error(w, fmt.Sprintf("플레이리스트 생성 실패: %v", err), http.StatusInternalServerError)
		return
	}

	// 등록할 3곡 목록
	targetTracks := []Track{
		{Artist: "아이유", Title: "밤편지"},
		{Artist: "헤이즈", Title: "비도 오고 그래서"},
		{Artist: "폴킴", Title: "모든 날 모든 순간"},
	}

	// 3곡 검색 후 순차 등록
	for i, track := range targetTracks {
		searchQuery := fmt.Sprintf("%s %s official audio", track.Artist, track.Title)
		videoID, err := searchFirstVideoID(youtubeService, searchQuery)
		if err != nil {
			log.Printf("[%d번 검색 실패] %s - %s: %v\n", i+1, track.Artist, track.Title, err)
			continue
		}

		playlistItem := &youtube.PlaylistItem{
			Snippet: &youtube.PlaylistItemSnippet{
				PlaylistId: createdPlaylist.Id,
				ResourceId: &youtube.ResourceId{
					Kind:    "youtube#video",
					VideoId: videoID,
				},
			},
		}

		_, err = youtubeService.PlaylistItems.Insert([]string{"snippet"}, playlistItem).Do()
		if err != nil {
			log.Printf("[%d번 트랙 등록 실패] %s (%s): %v\n", i+1, track.Title, videoID, err)
			continue
		}

		log.Printf("✅ [%d/3] %s - %s (ID: %s) 등록 완료", i+1, track.Artist, track.Title, videoID)
	}

	// 완성된 유튜브 뮤직 플레이리스트로 이동
	youtubeMusicURL := fmt.Sprintf("https://music.youtube.com/playlist?list=%s", createdPlaylist.Id)
	http.Redirect(w, r, youtubeMusicURL, http.StatusSeeOther)
}

// 검색어로 첫 번째 동영상 ID 추출
func searchFirstVideoID(service *youtube.Service, query string) (string, error) {
	call := service.Search.List([]string{"id"}).
		Q(query).
		Type("video").
		MaxResults(1)

	resp, err := call.Do()
	if err != nil {
		return "", err
	}

	if len(resp.Items) == 0 {
		return "", fmt.Errorf("검색 결과 없음")
	}

	return resp.Items[0].Id.VideoId, nil
}