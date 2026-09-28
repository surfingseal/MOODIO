package model

import "testing"

func TestTrack_SearchQuery(t *testing.T) {
	track := Track{
		Artist: "아이유",
		Title:  "밤편지",
	}

	expected := "아이유 밤편지 Official Audio"
	actual := track.SearchQuery()

	if actual != expected {
		t.Errorf("기대값 %q, 실제값 %q", expected, actual)
	}
}

func TestTrack_String(t *testing.T) {
	track := Track{
		Artist: "헤이즈",
		Title:  "비도 오고 그래서",
	}

	expected := "헤이즈 - 비도 오고 그래서"
	actual := track.String()

	if actual != expected {
		t.Errorf("기대값 %q, 실제값 %q", expected, actual)
	}
}
