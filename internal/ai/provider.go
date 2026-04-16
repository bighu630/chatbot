package ai

type AiInterface interface {
	Name() string
	HandleText(string) (string, error)
	HandleTextWithImg(msg string, imgType string, imgData []byte) (string, error)
	Chat(chatId string, msg string) (string, error)
	ChatWithImg(chatId string, msg string, imgType string, imgData []byte) (string, error)
	AddChatMsg(chatId string, userSay string, botSay string) error
	Translate(text string) (string, error)
}

type EmotionScores struct {
	Joy      float64 `json:"joy"`
	Anger    float64 `json:"anger"`
	Sadness  float64 `json:"sadness"`
	Fear     float64 `json:"fear"`
	Disgust  float64 `json:"disgust"`
	Surprise float64 `json:"surprise"`
}

type EmotionSearchParams struct {
	Scores      EmotionScores `json:"scores"`
	TopK        int           `json:"top_k"`
	MaxDistance float64       `json:"max_distance"`
	Source      string        `json:"source"`
	Tags        []string      `json:"tags"`
}

type EmotionSearchBuilder interface {
	BuildEmotionSearchParams(chatContext string, userMessage string, botReply string) (EmotionSearchParams, error)
}
