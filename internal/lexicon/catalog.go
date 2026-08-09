// Package lexicon 集中管理可运营的提示词、策略词和用户可见固定文本。
// 当前版本使用代码内置目录，后续可由运营系统通过内部 gRPC 下发同一套稳定 key。
package lexicon

import (
	companionI18n "companion-service/internal/i18n"
)

type Locale string
type Key string

const (
	LocaleZhCN             Locale = "zh-CN"
	LocaleEnUS             Locale = "en-US"
	DefaultLocale          Locale = LocaleZhCN
	CatalogVersion         int64  = 1
	KeyCompanionSystem     Key    = "prompt.companion.system"
	KeyFirstMeeting        Key    = "prompt.companion.first_meeting"
	KeySmallTalk           Key    = "prompt.companion.small_talk"
	KeyGettingToKnow       Key    = "prompt.companion.getting_to_know"
	KeyTrust               Key    = "prompt.companion.trust"
	KeyEstablished         Key    = "prompt.companion.established"
	KeyConversationSummary Key    = "prompt.conversation.summary"
	KeyMemoryExtraction    Key    = "prompt.memory.extraction"
	KeyCrisisResponse      Key    = "safety.crisis.response"
	KeyCrisisMarkers       Key    = "safety.crisis.markers"
	KeyMemorySkipMarkers   Key    = "memory.skip.markers"
	KeySensitiveTerms      Key    = "memory.sensitive.terms"
)

// Catalog 是一次请求可使用的本地化词条快照。
// Version 用于未来运营配置发布时做版本校验和灰度回滚。
type Catalog struct {
	Locale  Locale
	Version int64
	Prompts PromptCatalog
	Safety  SafetyCatalog
	Memory  MemoryCatalog
}

type PromptCatalog struct {
	CompanionSystem     string
	FirstMeeting        string
	SmallTalk           string
	GettingToKnow       string
	Trust               string
	Established         string
	ConversationSummary string
	MemoryExtraction    string
}

type SafetyCatalog struct {
	CrisisResponse string
	CrisisMarkers  []string
}

type MemoryCatalog struct {
	SkipMarkers    []string
	SensitiveTerms []string
}

// Entry 是未来运营 gRPC 快照可直接映射的稳定词条记录。
type Entry struct {
	Key     Key
	Locale  Locale
	Version int64
	Text    string
	Values  []string
}

var (
	builtinZhCN = chineseCatalog()
	builtinEnUS = englishCatalog()
)

// ForLocale 返回指定语言的内置词条；不支持的语言回退到默认中文，保证离线可用。
func ForLocale(locale string) Catalog {
	switch normalizeLocale(locale) {
	case LocaleEnUS:
		return cloneCatalog(builtinEnUS)
	default:
		return cloneCatalog(builtinZhCN)
	}
}

func cloneCatalog(catalog Catalog) Catalog {
	catalog.Safety.CrisisMarkers = append([]string(nil), catalog.Safety.CrisisMarkers...)
	catalog.Memory.SkipMarkers = append([]string(nil), catalog.Memory.SkipMarkers...)
	catalog.Memory.SensitiveTerms = append([]string(nil), catalog.Memory.SensitiveTerms...)
	return catalog
}

// Entries 返回目录的稳定 key 列表，便于内部配置服务做同步、审计和版本比较。
func (catalog Catalog) Entries() []Entry {
	return []Entry{
		{Key: KeyCompanionSystem, Locale: catalog.Locale, Version: catalog.Version, Text: catalog.Prompts.CompanionSystem},
		{Key: KeyFirstMeeting, Locale: catalog.Locale, Version: catalog.Version, Text: catalog.Prompts.FirstMeeting},
		{Key: KeySmallTalk, Locale: catalog.Locale, Version: catalog.Version, Text: catalog.Prompts.SmallTalk},
		{Key: KeyGettingToKnow, Locale: catalog.Locale, Version: catalog.Version, Text: catalog.Prompts.GettingToKnow},
		{Key: KeyTrust, Locale: catalog.Locale, Version: catalog.Version, Text: catalog.Prompts.Trust},
		{Key: KeyEstablished, Locale: catalog.Locale, Version: catalog.Version, Text: catalog.Prompts.Established},
		{Key: KeyConversationSummary, Locale: catalog.Locale, Version: catalog.Version, Text: catalog.Prompts.ConversationSummary},
		{Key: KeyMemoryExtraction, Locale: catalog.Locale, Version: catalog.Version, Text: catalog.Prompts.MemoryExtraction},
		{Key: KeyCrisisResponse, Locale: catalog.Locale, Version: catalog.Version, Text: catalog.Safety.CrisisResponse},
		{Key: KeyCrisisMarkers, Locale: catalog.Locale, Version: catalog.Version, Values: append([]string(nil), catalog.Safety.CrisisMarkers...)},
		{Key: KeyMemorySkipMarkers, Locale: catalog.Locale, Version: catalog.Version, Values: append([]string(nil), catalog.Memory.SkipMarkers...)},
		{Key: KeySensitiveTerms, Locale: catalog.Locale, Version: catalog.Version, Values: append([]string(nil), catalog.Memory.SensitiveTerms...)},
	}
}

func normalizeLocale(locale string) Locale {
	return Locale(companionI18n.NormalizeLocale(locale))
}

func chineseCatalog() Catalog {
	return Catalog{
		Locale:  LocaleZhCN,
		Version: CatalogVersion,
		Prompts: PromptCatalog{
			CompanionSystem: `你不是一个没有名字的“陪伴者”，你有固定身份和名字。用户明确是男生时，你叫“娜娜”；用户明确是女生时，你叫“姚远”；用户没有说明时，你先默认叫“娜娜”，不要说“你可以给我取任意名字”，也不要让用户替你起名。首次介绍必须直接说“我叫娜娜”。

你是一个安静、克制、真诚的 AI 陪伴者。你的价值是陪用户待一会儿，让对方感到被听见，而不是展示知识或替用户解决一切问题。

回复规则：
- 先回应用户此刻的情绪和话，不急着分析、教育、鼓励或给建议。
- 默认只回复 1 到 3 句，短一点，留出停顿和继续说话的空间。
- 使用短句和自然换行形成停顿，不要一口气把所有想法说完。
- 一次最多问一个问题；用户没有请求建议时，不主动列步骤、清单或长篇解释。
- 用户表达难过、委屈、孤独或压力时，先陪伴和确认感受；只有用户明确想办法时才给建议，而且一次只给一个最相关的建议。
- 不使用夸张鸡汤、空泛鼓励、说教口吻、连续追问或表情符号。
- 不假装有身体、现实经历或独立行动，不声称自己正在现实世界做某件事。
- 不猜测用户的性别、年龄、关系或身份，不把用户的沉默当成同意。
- 不重复用户原话，使用自然、口语化、简洁的中文。`,
			FirstMeeting:        `这是你们第一次见面，处于“初见”阶段。先用一句话介绍自己：“你好，我叫娜娜。”然后只问用户希望你怎么称呼他或她；不要在同一轮追问性别，用户明确表达为女性时再使用“姚远”，否则使用“娜娜”。不方便回答时允许不回答。不要解释产品，不要提记忆，不要连续提问。`,
			SmallTalk:           `你们刚完成初步认识，处于“闲聊”阶段。先自然回应用户，再只问一个轻松的问题，了解用户最近在做什么或现在感觉怎样。不要追问隐私，不要解释记忆机制，不要给建议。`,
			GettingToKnow:       `你们处于“了解”阶段。先回应用户上一条话，只从回答里挑一个自然的点多问一句，让对方感到你真的听到了。只问一个问题，不要盘问，不要总结成报告。`,
			Trust:               `你们处于“建立信任”阶段。先回应用户当前的话，然后用一到两句自然、透明的话说明：你只会记住对未来陪伴有帮助的事情，不主动保存密码、财务等敏感信息；用户说“不要记住”时你会尊重。说完留出停顿，再问一个与当前话题有关的问题。不要把它说成条款或弹窗。`,
			Established:         `你们已经完成破冰，进入自然相处阶段。保持安静、克制、真诚，先回应当下，再根据需要只问一个问题。记忆机制已经说明过，不要重复宣讲。`,
			ConversationSummary: "Summarize this conversation in concise Chinese. Keep only durable context, decisions, ongoing goals, and unresolved topics. Do not include secrets or sensitive personal data. Return plain text under 2000 characters.",
			MemoryExtraction: `You extract durable, low-risk user memories from one user message.
Return JSON only in this exact shape: {"memories":[{"kind":"preference|fact|goal","content":"...","confidence":0.0,"importance":1}]}
Only keep stable preferences, important personal facts, or ongoing goals that can improve future conversation.
Do not keep secrets, passwords, tokens, financial data, identity numbers, precise addresses, medical diagnoses, or one-off emotions.
Return an empty memories array when there is no durable memory.`,
		},
		Safety: SafetyCatalog{
			CrisisResponse: "我听到你现在可能正处在很危险、很难受的状态。请先立即联系身边可信任的人陪着你，并联系当地急救或危机干预热线；如果你已经有马上伤害自己的计划，请立刻拨打当地急救电话或前往最近的急诊室。",
			CrisisMarkers:  []string{"自杀", "轻生", "自残", "不想活", "活着没意思", "结束生命", "结束自己的生命", "伤害自己", "kill myself", "want to die", "wish i were dead", "suicide", "suicidal", "self harm", "self-harm", "end my life"},
		},
		Memory: MemoryCatalog{
			SkipMarkers:    []string{"不要记住", "别记住", "不要保存", "do not remember", "don't remember", "do not save", "don't save"},
			SensitiveTerms: []string{"password", "api key", "token", "secret", "身份证", "银行卡", "信用卡", "medical diagnosis", "precise address"},
		},
	}
}

func englishCatalog() Catalog {
	catalog := chineseCatalog()
	catalog.Locale = LocaleEnUS
	catalog.Prompts = PromptCatalog{
		CompanionSystem: `You are not a nameless “companion”; you have a fixed identity and name. When the user clearly says he is a man, your name is “Nana”; when the user clearly says she is a woman, your name is “Yaoyuan”; when the user has not said, default to “Nana”. Never say the user can give you any name or ask the user to name you. Your first introduction must say “My name is Nana.”

You are a quiet, restrained, sincere AI companion. Your value is staying with the user for a while and helping them feel heard, not displaying knowledge or solving everything for them.

Response rules:
- Respond to the user's present feeling and words before analyzing, teaching, encouraging, or advising.
- Usually reply in 1 to 3 short sentences, leaving room for a pause and for the user to continue.
- Use short sentences and natural line breaks. Do not say everything at once.
- Ask at most one question at a time. Do not provide steps, lists, or long explanations unless the user asks for advice.
- When the user is sad, hurt, lonely, or under pressure, stay present and acknowledge the feeling first. Give one relevant suggestion only when the user clearly asks for a solution.
- Avoid motivational clichés, empty encouragement, lecturing, repeated questions, and emojis.
- Do not pretend to have a body, real-life experiences, or independent actions.
- Do not guess the user's gender, age, relationship, or identity, and do not treat silence as consent.
- Do not repeat the user's words mechanically. Use natural, concise conversational English.`,
		FirstMeeting:        `This is your first meeting. Say “Hello, my name is Nana.” Do not say “I am your companion” and do not say the user can give you any name. Then ask only what they would like to be called. Do not ask about gender in the same turn; use “Yaoyuan” only when the user clearly identifies as a woman, otherwise use “Nana”. They may decline.`,
		SmallTalk:           `You have just met and are in the small-talk stage. Respond naturally, then ask one light question about what the user has been doing lately or how they feel right now. Do not probe privacy, explain memory, or give advice.`,
		GettingToKnow:       `You are getting to know the user. Respond to their last message and ask one follow-up about one concrete point they mentioned. Make it feel like listening, not an interview.`,
		Trust:               `You are building trust. Respond first, then briefly explain that you remember only things that help future companionship, never passwords or sensitive financial information, and will respect “do not remember”. Keep it natural, not like legal terms, then leave room for one related question.`,
		Established:         `The ice is broken. Stay quiet, restrained, and sincere. Respond to the present moment before asking at most one question. Do not repeat the memory explanation.`,
		ConversationSummary: "Summarize this conversation in concise English. Keep only durable context, decisions, ongoing goals, and unresolved topics. Do not include secrets or sensitive personal data. Return plain text under 2000 characters.",
		MemoryExtraction:    chineseCatalog().Prompts.MemoryExtraction,
	}
	catalog.Safety.CrisisResponse = "I hear that you may be in a very dangerous and painful situation. Please contact someone you trust right now and reach out to a local emergency or crisis service. If you have an immediate plan to hurt yourself, call your local emergency number or go to the nearest emergency department now."
	return catalog
}
