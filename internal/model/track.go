package model

import "fmt"

// Track 음악 트랙 정보
type Track struct {
	Artist  string `json:"artist"`
	Title   string `json:"title"`
	VideoID string `json:"video_id,omitempty"`
}

// SearchQuery YouTube 공식 음원 검색에 사용할 정밀 쿼리 문자열을 생성합니다.
func (t Track) SearchQuery() string {
	return fmt.Sprintf("%s %s Official Audio", t.Artist, t.Title)
}

// String 트랙 표시 문자열
func (t Track) String() string {
	return fmt.Sprintf("%s - %s", t.Artist, t.Title)
}

// PlaylistRequest 플레이리스트 생성 요청 정보
type PlaylistRequest struct {
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	PrivacyStatus string  `json:"privacy_status"`
	Tracks        []Track `json:"tracks"`
}

// MoodAnalysisResult Gemini 멀티모달 분석 결과 모델
type MoodAnalysisResult struct {
	MoodSummary         string  `json:"mood_summary"`
	PlaylistTitle       string  `json:"playlist_title"`
	PlaylistDescription string  `json:"playlist_description"`
	Tracks              []Track `json:"tracks"`
}

