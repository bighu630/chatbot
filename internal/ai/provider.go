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
