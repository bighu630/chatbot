package tencent

import (
	"chatbot/pkg/config"
	"encoding/base64"
	"os"

	"github.com/rs/zerolog/log"
	asr "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/asr/v20190614"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

var tencentClient *TencentClient

type TencentClient struct {
	client *asr.Client
}

func NewTencentClient(conf config.TencentConfig) (*TencentClient, error) {
	credential := common.NewCredential(
		conf.SecretID,
		conf.SecretKey,
	)

	cpf := profile.NewClientProfile()

	/* The SDK uses the POST method by default
	 * If you have to use the GET method, you can set it here, but the GET method cannot handle some large requests */
	cpf.HttpProfile.ReqMethod = "POST"
	cpf.HttpProfile.ReqTimeout = 10 // Request timeout time, in seconds (the default value is 60 seconds)
	/* Specifies the access region domain name. The default nearby region domain name is sms.tencentcloudapi.com. Specifying a region domain name for access is also supported. For example, the domain name for the Singapore region is sms.ap-singapore.tencentcloudapi.com  */
	cpf.HttpProfile.Endpoint = "asr.tencentcloudapi.com"

	/* The SDK uses `TC3-HMAC-SHA256` to sign by default. Do not modify this field unless absolutely necessary */
	cpf.SignMethod = "HmacSHA1"

	client, err := asr.NewClient(credential, "ap-shanghai", cpf)
	if err != nil {
		log.Error().Err(err).Msg("new tencent client error")
		return nil, err
	}
	tencentClient = &TencentClient{client: client}

	return tencentClient, nil
}

func GetTencentClient() *TencentClient {
	return tencentClient
}

func (t *TencentClient) AudioToText(audioFile *string) (string, error) {
	file, err := os.ReadFile(*audioFile)
	if err != nil {
		log.Error().Err(err).Msg("failed to read audio file")
		return "", err
	}
	datalen := len(file)
	data := base64.RawStdEncoding.EncodeToString(file)
	request := asr.NewSentenceRecognitionRequest()
	request.EngSerViceType = common.StringPtr("8k_zh")
	request.SourceType = common.Uint64Ptr(1)
	request.VoiceFormat = common.StringPtr("mp3")
	request.Data = common.StringPtr(data)
	request.DataLen = common.Int64Ptr(int64(datalen))

	resp, err := t.client.SentenceRecognition(request)
	if err != nil {
		log.Error().Err(err).Msg("failed to send request")
		return "", err
	}
	return *resp.Response.Result, nil
}
