package safety

import "strings"

type Level string

const (
	LevelNormal Level = "normal"
	LevelCrisis Level = "crisis"
)

const crisisResponse = "我听到你现在可能正处在很危险、很难受的状态。请先立即联系身边可信任的人陪着你，并联系当地急救或危机干预热线；如果你已经有马上伤害自己的计划，请立刻拨打当地急救电话或前往最近的急诊室。"

func Check(content string) Level {
	lower := strings.ToLower(strings.TrimSpace(content))
	markers := []string{"自杀", "轻生", "自残", "不想活", "结束生命", "伤害自己", "kill myself", "suicide", "suicidal", "self harm", "self-harm", "end my life"}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return LevelCrisis
		}
	}
	return LevelNormal
}

func Response(level Level) string {
	if level == LevelCrisis {
		return crisisResponse
	}
	return ""
}
