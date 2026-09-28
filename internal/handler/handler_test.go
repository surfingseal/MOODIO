package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/surfingseal/MOODIO/internal/config"
	"github.com/surfingseal/MOODIO/internal/model"
)

func TestPendingOAuthCookie(t *testing.T) {
	cfg := &config.Config{
		OAuthState: "test-secret-state-12345",
	}
	h := New(cfg, nil)

	reqData := model.PlaylistRequest{
		Title:         "테스트 플레이리스트 🎵",
		Description:   "설명",
		PrivacyStatus: "public",
		Tracks: []model.Track{
			{Artist: "아이유", Title: "밤편지"},
			{Artist: "뉴진스", Title: "Ditto"},
		},
	}

	state := generateRandomState()

	// 1. 쿠키 설정 테스트
	rec := httptest.NewRecorder()
	httpReq := httptest.NewRequest("POST", "/api/create-flow", nil)

	err := h.setPendingOAuthCookie(rec, httpReq, state, reqData)
	if err != nil {
		t.Fatalf("setPendingOAuthCookie error: %v", err)
	}

	cookie := rec.Result().Cookies()
	if len(cookie) == 0 {
		t.Fatalf("expected cookie to be set, got none")
	}

	// 2. 쿠키 읽기 및 검증 테스트
	callbackReq := httptest.NewRequest("GET", "/auth/google/callback", nil)
	callbackReq.AddCookie(cookie[0])

	cbRec := httptest.NewRecorder()
	recovered, ok := h.getPendingOAuthSession(cbRec, callbackReq, state)
	if !ok {
		t.Fatalf("expected getPendingOAuthSession to succeed, failed")
	}

	if recovered.Title != reqData.Title {
		t.Errorf("expected title %s, got %s", reqData.Title, recovered.Title)
	}
	if len(recovered.Tracks) != len(reqData.Tracks) {
		t.Errorf("expected %d tracks, got %d", len(reqData.Tracks), len(recovered.Tracks))
	}

	// 3. 변조된 State 검증 실패 테스트
	mismatchedReq := httptest.NewRequest("GET", "/auth/google/callback", nil)
	mismatchedReq.AddCookie(cookie[0])

	_, ok = h.getPendingOAuthSession(httptest.NewRecorder(), mismatchedReq, "wrong-state")
	if ok {
		t.Errorf("expected getPendingOAuthSession to fail with wrong state")
	}
}
