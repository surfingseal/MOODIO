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
		modelName = "gemini-3.8-flash"
	}

	// 기본 멀티모달 고성능 플래그십 모델로 gemini-3.8-flash 사용
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
				Description: "사진의 분위기와 완벽히 어울리는 대표 한국/외국 음악 3~5곡",
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"artist": {Type: genai.TypeString, Description: "가수 또는 그룹명"},
						"title":  {Type: genai.TypeString, Description: "노래 제목"},
					},
					Required: []string{"artist", "title"},
				},
			},
		},
		Required: []string{"mood_summary", "playlist_title", "playlist_description", "tracks"},
	}

	prompt := `당신은 사진의 분위기를 읽어내어 완벽한 음악 플레이리스트를 만들어주는 전문 DJ이자 음악 큐레이터입니다.
제공된 이미지를 정밀하게 분석하여:
1. 사진의 계절, 날씨, 조명, 장소, 피사체, 전체적인 감정/분위기를 파악하세요.
2. 이 분위기와 완벽하게 조화되는 인기 있고 검증된 대표 음악 3~5곡(국내 가요, 인디, 팝 등)을 엄선하세요.
3. YouTube에서 공식 음원으로 정확히 검색될 수 있는 정확하고 정식 표기된 아티스트명과 곡명을 제공하세요.
반드시 지정된 JSON 규격에 맞추어 한국어로 답변하세요.`

	parts := []*genai.Part{
		{InlineData: &genai.Blob{Data: imageBytes, MIMEType: mimeType}},
		{Text: prompt},
	}
	contents := []*genai.Content{{Parts: parts}}

	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   schema,
		Temperature:      genai.Ptr[float32](0.7),
		MaxOutputTokens:  800,
		// 직관적인 감성 선곡 작업을 위해 Thinking을 Low로 설정하여 응답 대기 시간을 최소화
		ThinkingConfig: &genai.ThinkingConfig{
			ThinkingLevel: genai.ThinkingLevelLow,
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
