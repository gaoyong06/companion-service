package safety

import (
	"strings"

	"companion-service/internal/lexicon"
)

type Level string

const (
	LevelNormal Level = "normal"
	LevelCrisis Level = "crisis"
)

func Check(content string) Level {
	return CheckForLocale(content, string(lexicon.DefaultLocale))
}

// CheckForLocale 使用指定语言目录识别危机表达。
func CheckForLocale(content, locale string) Level {
	lower := strings.ToLower(strings.TrimSpace(content))
	for _, marker := range lexicon.ForLocale(locale).Safety.CrisisMarkers {
		if strings.Contains(lower, marker) {
			return LevelCrisis
		}
	}
	return LevelNormal
}

func Response(level Level) string {
	return ResponseForLocale(level, string(lexicon.DefaultLocale))
}

// ResponseForLocale 返回指定语言的安全响应，缺失语言时回退默认词条。
func ResponseForLocale(level Level, locale string) string {
	if level == LevelCrisis {
		return lexicon.ForLocale(locale).Safety.CrisisResponse
	}
	return ""
}
