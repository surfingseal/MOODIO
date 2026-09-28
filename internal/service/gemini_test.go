package service

import (
	"context"
	"os"
	"testing"
)

func TestAnalyzeMoodFromImage(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set")
	}

	svc, err := NewGeminiService(context.Background(), apiKey, "gemini-3.5-flash-lite")
	if err != nil {
		t.Fatalf("NewGeminiService error: %v", err)
	}

	imgBytes, err := os.ReadFile("/Users/surfing_seal/.gemini/antigravity-ide/brain/0f8bdcc9-3abd-4c36-839f-78937c495e5b/.user_uploaded/media_1790571724437.png")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	result, err := svc.AnalyzeMoodFromImage(context.Background(), imgBytes, "image/png")
	if err != nil {
		t.Fatalf("AnalyzeMoodFromImage failed: %v", err)
	}

	t.Logf("Result: MoodSummary=%s, Title=%s, Tracks=%d", result.MoodSummary, result.PlaylistTitle, len(result.Tracks))
	for _, tr := range result.Tracks {
		t.Logf(" - %s by %s", tr.Title, tr.Artist)
	}
}
