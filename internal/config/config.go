package config

import (
	"fmt"
	"log"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Config 애플리케이션 실행 환경 설정
type Config struct {
	Port               string
	GoogleClientID     string
	GoogleClientSecret string
	RedirectURL        string
	OAuthState         string
	GeminiAPIKey       string
	GeminiModel        string
}

// Load 환경변수로부터 설정을 로드하고 검증합니다.
func Load() (*Config, error) {
	cfg := &Config{
		Port:               getEnv("PORT", "8080"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:        getEnv("REDIRECT_URL", "http://localhost:8080/auth/google/callback"),
		OAuthState:         getEnv("OAUTH_STATE", "moodio-oauth-state-token"),
		GeminiAPIKey:       os.Getenv("GEMINI_API_KEY"),
		GeminiModel:        getEnv("GEMINI_MODEL", "gemini-3.8-flash"),
	}

	if cfg.GeminiAPIKey == "" {
		log.Println("⚠️ [경고] GEMINI_API_KEY가 설정되지 않았습니다.")
		log.Println("   사진 분위기 분석 AI 기능을 사용하려면 GEMINI_API_KEY를 설정해주세요.")
	}

	if cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
		log.Println("⚠️ [경고] GOOGLE_CLIENT_ID 또는 GOOGLE_CLIENT_SECRET이 설정되지 않았습니다.")
		log.Println("   YouTube OAuth 인증을 진행하려면 환경변수를 설정해주세요.")
	}

	return cfg, nil
}

// OAuth2Config Google OAuth2 설정을 반환합니다.
func (c *Config) OAuth2Config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.GoogleClientID,
		ClientSecret: c.GoogleClientSecret,
		RedirectURL:  c.RedirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/youtube",
			"https://www.googleapis.com/auth/userinfo.email",
		},
		Endpoint: google.Endpoint,
	}
}

// Addr HTTP 서버 리슨 주소 문자열 반환
func (c *Config) Addr() string {
	return fmt.Sprintf(":%s", c.Port)
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}
