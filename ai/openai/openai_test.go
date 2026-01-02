package openai

import (
	"chatbot/config"
	"testing"
)

var openaiInstance *openAi

func init() {
	// envs, err := godotenv.Read("../../.env")
	// if err != nil {
	// 	panic("Error loading .env file")
	// }

	// err = config.LoadConfig("../../config.toml")
	// if err != nil {
	// 	panic(err)
	// }
	// config.GlobalConfig.Ai.OpenAiKey = envs["OPENAI_API_KEY"]
	// config.GlobalConfig.Ai.OpenAiModel = envs["OPENAI_MODEL"]
	// config.GlobalConfig.Ai.OpenAiBaseUrl = envs["OPENAI_URL"]
	config.GlobalConfig.Ai.OpenAiBaseUrl = "https://api.deepseek.com"
	config.GlobalConfig.Ai.OpenAiKey = "sk-2f009aec9e5e4eba865ce687efd9f0db"
	config.GlobalConfig.Ai.OpenAiModel = "deepseek-chat"
	openaiInstance = NewOpenAi(config.GlobalConfig.Ai)
}

func TestOpenAi_HandleText(t *testing.T) {
	if openaiInstance == nil {
		t.Fatal("openaiInstance is nil")
	}
	resp, err := openaiInstance.HandleText("你是谁")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp)
}

func TestOpenAi_Chat(t *testing.T) {
	if openaiInstance == nil {
		t.Fatal("openaiInstance is nil")
	}
	resp, err := openaiInstance.Chat("123", `看得见这个图片吗
对话包含图片内容这张图片的主体是一只浅灰色的猫，它拥有非常显著的异色瞳：从观察者角度看，它的左眼是蓝色，右眼是绿色。它的鼻子是粉红色的。

这只猫直立着，面向镜头，站在几块不规则的、颜色偏深灰且表面粗糙的水泥块或砖头上。

猫的背景是一面灰色的、纹理清晰的墙壁，可能是水泥或抹灰墙。墙壁占据了画面上方和左侧的大部分区域。

在猫的右侧和下方，以及水泥块周围，散落着一堆干枯的树枝或竹竿。地面上还能看到一些枯叶和杂物。

图片的光线非常明亮，阳光强烈，从左上方照射过来，在地面和物体上投下了清晰而浓重的阴影。整个场景呈现出一种户外、自然光下的景象，背景略显斑驳和荒芜。`)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp)
}
func TestOpenAi_Chat_With_Context(t *testing.T) {
	if openaiInstance == nil {
		t.Fatal("openaiInstance is nil")
	}
	resp, err := openaiInstance.Chat("123", "记住你最喜欢的水果是桃子")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp)
	resp, err = openaiInstance.Chat("123", "你最喜欢的水果是什么")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp)
}

func TestOpenAi_AddChatMsg(t *testing.T) {
	if openaiInstance == nil {
		t.Fatal("openaiInstance is nil")
	}
	err := openaiInstance.AddChatMsg("123", "hello", "hi")
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenAi_Translate(t *testing.T) {
	if openaiInstance == nil {
		t.Fatal("openaiInstance is nil")
	}
	_, err := openaiInstance.Translate("hello")
	if err == nil {
		t.Fatal("err is nil")
	}
}
