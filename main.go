package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/surfingseal/MOODIO/internal/config"
	"github.com/surfingseal/MOODIO/internal/handler"
	"github.com/surfingseal/MOODIO/internal/service"
)

func main() {
	// 1. 설정 로드
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("설정 로드 실패: %v", err)
	}

	// 2. Gemini 멀티모달 서비스 초기화 (API Key가 설정된 경우)
	var geminiService *service.GeminiService
	if cfg.GeminiAPIKey != "" {
		ctx := context.Background()
		geminiService, err = service.NewGeminiService(ctx, cfg.GeminiAPIKey, cfg.GeminiModel)
		if err != nil {
			log.Printf("⚠️ Gemini 서비스 초기화 실패: %v", err)
		} else {
			log.Printf("✨ Gemini 멀티모달 AI 서비스 초기화 완료 (모델: %s)\n", cfg.GeminiModel)
		}
	} else {
		log.Println("ℹ️ GEMINI_API_KEY가 설정되지 않아 사진 분석 시 오류 안내가 반환됩니다.")
	}

	// 3. 라우터 및 핸들러 등록
	mux := http.NewServeMux()
	h := handler.New(cfg, geminiService)
	h.RegisterRoutes(mux)

	// 4. HTTP 서버 설정
	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 5. Graceful Shutdown 채널 설정
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	// 서버 비동기 실행
	go func() {
		fmt.Printf("🚀 MOODIO 서버 실행 중: http://localhost:%s\n", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("서버 실행 오류: %v", err)
		}
	}()

	// 종료 시그널 대기
	<-stopChan
	log.Println("🛑 서버 종료 시그널 수신, 종료 절차 시작...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("서버 강제 종료: %v", err)
	} else {
		log.Println("✅ 서버가 정상적으로 안전하게 종료되었습니다.")
	}
}