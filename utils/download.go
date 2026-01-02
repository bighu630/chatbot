package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/google/uuid"
)

var baseUrl = "https://api.telegram.org/bot"

var cli client

type client struct {
	httpClient *http.Client
}

func init() {
	cli = client{
		httpClient: &http.Client{},
	}
}

func (c *client) sendRequest(req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req)
}

// return imgtype,data,err
func DownloadImgByFileID(fileID string, b *gotgbot.Bot) (string, []byte, error) {
	url := fmt.Sprintf("%s%s/getFile?file_id=%s", baseUrl, b.Token, fileID)
	filePathReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := cli.sendRequest(filePathReq)
	if err != nil {
		return "", nil, err
	}
	var data map[string]any
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return "", nil, err
	}
	filePath := data["result"].(map[string]any)["file_path"].(string)

	if filePath == "" {
		return "", nil, errors.New("failed to get file path")
	}
	ext := filepath.Ext(filePath) // 得到 ".jpg"
	if len(ext) > 0 {
		ext = ext[1:] // 去掉前面的点
	}
	url = fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.Token, filePath)
	fileReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err = cli.sendRequest(fileReq)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return "", nil, err
	}
	imgData := buf.Bytes()
	return ext, imgData, nil
}

func DownloadAduioByFileID(fileID string, b *gotgbot.Bot) (*string, error) {
	url := fmt.Sprintf("%s%s/getFile?file_id=%s", baseUrl, b.Token, fileID)
	filePathReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := cli.sendRequest(filePathReq)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return nil, err
	}
	filePath := data["result"].(map[string]any)["file_path"].(string)

	if filePath == "" {
		return nil, errors.New("failed to get file path")
	}
	url = fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.Token, filePath)
	fileReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err = cli.sendRequest(fileReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	filePath = filepath.Join("/tmp", "temp"+uuid.NewString())
	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return nil, err
	}
	oggFilePath := filepath.Join("/tmp", "temp"+uuid.NewString()+".mp3")
	err = convertOgaToOggOpus(filePath, oggFilePath)
	if err != nil {
		return nil, err
	}
	return &oggFilePath, nil
}

func convertOgaToOggOpus(inputFile, outputFile string) error {
	// 检查 ffmpeg 是否已安装
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found: %v", err)
	}

	// 构建 ffmpeg 命令，将 .oga 文件转换为 .ogg（opus 编码）
	cmd := exec.Command("ffmpeg", "-i", inputFile, outputFile)

	// 设置输出到标准输出和标准错误
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 运行命令
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to convert file: %v", err)
	}

	fmt.Printf("File converted successfully to %s\n", outputFile)
	return nil
}

func EscapeMarkdownChars(input string) string {
	// 定义需要转义的特殊字符
	specialChars := "_*[]()~`>#+-=|{}.!"
	// 创建一个 Builder 来高效拼接字符串
	var builder strings.Builder
	for _, char := range input {
		if strings.ContainsRune(specialChars, char) {
			builder.WriteRune('\\') // 添加反斜杠
		}
		builder.WriteRune(char) // 添加字符
	}
	return builder.String()
}
