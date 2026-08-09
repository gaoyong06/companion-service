package biz

import "strings"

const (
	OnboardingStageFirstMeeting  = "first_meeting"
	OnboardingStageSmallTalk     = "small_talk"
	OnboardingStageGettingToKnow = "getting_to_know"
	OnboardingStageTrust         = "trust"
	OnboardingStageEstablished   = "established"
)

func normalizeOnboardingStage(stage string) string {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case OnboardingStageFirstMeeting, OnboardingStageSmallTalk, OnboardingStageGettingToKnow, OnboardingStageTrust, OnboardingStageEstablished:
		return strings.ToLower(strings.TrimSpace(stage))
	default:
		return OnboardingStageFirstMeeting
	}
}

func nextOnboardingStage(stage string) string {
	switch normalizeOnboardingStage(stage) {
	case OnboardingStageFirstMeeting:
		return OnboardingStageSmallTalk
	case OnboardingStageSmallTalk:
		return OnboardingStageGettingToKnow
	case OnboardingStageGettingToKnow:
		return OnboardingStageTrust
	default:
		return OnboardingStageEstablished
	}
}
