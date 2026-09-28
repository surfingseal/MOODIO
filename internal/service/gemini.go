package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/surfingseal/MOODIO/internal/model"
	"google.golang.org/genai"
)

// GeminiService Google Gen AI SDK를 활용한 멀티모달 분석 서비스
type GeminiService struct {
	client *genai.Client
	model  string
}

// NewGeminiService Gemini 클라이언트를 초기화합니다.
func NewGeminiService(ctx context.Context, apiKey, modelName string) (*GeminiService, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY가 설정되지 않았습니다")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("Gemini 클라이언트 생성 실패: %w", err)
	}

	if modelName == "" {
		modelName = "gemini-3.5-flash-lite"
	}

	// 초저지연 멀티모달 경량 모델로 gemini-3.5-flash-lite 사용
	return &GeminiService{
		client: client,
		model:  modelName,
	}, nil
}

// AnalyzeMoodFromImage 업로드된 사진을 분석하여 분위기와 어울리는 음악 목록을 추천합니다.
func (s *GeminiService) AnalyzeMoodFromImage(ctx context.Context, imageBytes []byte, mimeType string) (*model.MoodAnalysisResult, error) {
	if len(imageBytes) == 0 {
		return nil, fmt.Errorf("이미지 데이터가 비어있습니다")
	}

	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	// 구조화된 JSON 응답 스키마 정의
	schema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"mood_summary": {
				Type:        genai.TypeString,
				Description: "사진의 분위기, 감정, 날씨, 장소 등을 감성적으로 요약한 문장 (한국어)",
			},
			"playlist_title": {
				Type:        genai.TypeString,
				Description: "이 사진과 어울리는 감성적인 플레이리스트 제목 (이모지 포함)",
			},
			"playlist_description": {
				Type:        genai.TypeString,
				Description: "플레이리스트에 대한 간단한 소개 및 설명 (한국어)",
			},
			"tracks": {
				Type:        genai.TypeArray,
				Description: "Melon, Spotify, YouTube Music에 정식 발매되어 실존하는 대표 유명 음악 3~5곡 (없는 곡 지어내기 절대 금지)",
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"artist": {Type: genai.TypeString, Description: "실존하는 공식 가수/그룹명 (예: 아이유, 헤이즈, Coldplay, 뉴진스)"},
						"title":  {Type: genai.TypeString, Description: "실제 발매된 정식 노래 제목 (예: 밤편지, 비도 오고 그래서, Yellow)"},
					},
					Required: []string{"artist", "title"},
				},
			},
		},
		Required: []string{"mood_summary", "playlist_title", "playlist_description", "tracks"},
	}

	prompt := `당신은 사진의 분위기를 정밀하게 읽어내어 완벽한 음악 플레이리스트를 만들어주는 전문 음악 큐레이터입니다.

[🚨 최우선 원칙: 없는 곡명/가수 지어내기(환각/Hallucination) 절대 엄금]
1. 절대로 존재하지 않는 가상의 곡이나, 분위기에 어울릴 것 같다고 임의로 창작한 곡명을 추천하지 마세요.
2. 반드시 Melon, Spotify, YouTube Music에 정식 발매되어 대중적으로 널리 알려진 실존하는 유명 가수의 '실제 대표 히트곡'만 추천해야 합니다.
3. 국내 가요는 멜론/유튜브뮤직에 등록된 정식 공식 한국어 표기(예: '아이유' - '밤편지', '헤이즈' - '비도 오고 그래서', '성시경' - '너의 모든 순간')를 사용하고, 팝송은 원래 정식 영문 표기(예: 'Coldplay' - 'Yellow')를 사용하세요.
4. 사진의 분위기, 계절감, 장소, 날씨, 감정을 완벽히 반영하되, YouTube에서 공식 음원(Official Audio)으로 즉시 검색 가능한 실존 명곡 3~5곡을 선정하세요.
반드시 지정된 JSON 규격에 맞추어 한국어로 답변하세요.`

	parts := []*genai.Part{
		{InlineData: &genai.Blob{Data: imageBytes, MIMEType: mimeType}},
		{Text: prompt},
	}
	contents := []*genai.Content{{Parts: parts}}

	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   schema,
		// 환각 방지(프롬프트)와 감성 선곡 다양성 간의 황금 밸런스를 위해 0.45로 설정
		Temperature:     genai.Ptr[float32](0.45),
		MaxOutputTokens: 800,
		// 직관적인 감성 선곡 작업을 위해 Thinking을 Minimal로 설정하여 추론 딜레이를 최소화하고 즉각 응답
		ThinkingConfig: &genai.ThinkingConfig{
			ThinkingLevel: genai.ThinkingLevelMinimal,
		},
	}

	log.Printf("🤖 Gemini 멀티모달 이미지 분석 시작 (모델: %s, 이미지 크기: %d bytes)", s.model, len(imageBytes))
	resp, err := s.client.Models.GenerateContent(ctx, s.model, contents, config)
	if err != nil {
		return nil, fmt.Errorf("Gemini 이미지 분석 요청 실패: %w", err)
	}

	rawText := resp.Text()
	if rawText == "" {
		return nil, fmt.Errorf("Gemini 응답이 비어있습니다")
	}

	var result model.MoodAnalysisResult
	if err := json.Unmarshal([]byte(rawText), &result); err != nil {
		return nil, fmt.Errorf("Gemini 응답 JSON 파싱 실패 (%s): %w", rawText, err)
	}

	log.Printf("✨ Gemini 분석 완료 - 분위기: %s, 추천곡: %d곡", result.MoodSummary, len(result.Tracks))
	return &result, nil
}
