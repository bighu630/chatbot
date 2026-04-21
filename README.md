# 摘星

一个基于 Telegram 的群聊机器人项目。

当前最重要的能力是 `chat`:
- 在群聊中和群友对话
- 结合最近群消息形成短上下文
- 以更像普通群友的方式回复

除聊天外，项目还包含：
- YouTube Music 链接下载
- 语录功能
- 入群验证
- 反馈转管理员
- 定时任务框架

## 当前状态

代码当前的实际行为和旧 README 有一些差异，以下说明以仓库里的实现为准。

- 聊天入口已经接入，主逻辑位于 `internal/handler/chat.go`
- AI provider 抽象存在，当前聊天实现实际走 OpenAI client
- 图片消息支持先做图像描述，再并入聊天输入
- Telegram 接入默认使用 webhook

## 核心功能

### 1. 群聊 AI 对话

支持以下触发方式：
- 私聊 bot
- 群聊中发送 `/chat <消息>`
- 在群里直接回复 bot 的消息
- 在群里以 `@bot` 开头提及 bot
- 极低概率随机插话

群管理员或机器人管理员可以用 `/activity 0-10` 设置当前群随机插话活跃度；也支持 `/setactivity 0-10`。`0` 表示关闭随机插话，`10` 表示基础概率放大 10 倍。

群管理员或机器人管理员可以用 `/nsfw 0|1|2` 设置当前群表情搜索的 NSFW 模式；也支持 `/setnsfw 0|1|2`。
- `0`: 只搜正常图，对应 `is_nsfw=false`
- `1`: 只搜 NSFW，对应 `is_nsfw=true`
- `2`: 不传 `is_nsfw`，允许混合结果

群聊普通消息在未触发聊天时会先进入短期缓存；一旦触发聊天，bot 会把最近一批群消息拼成上下文，连同新消息一起发给 AI。

### 2. 图片辅助聊天

如果当前消息或被回复消息里包含图片，程序会先调用 Gemini 做图片描述，再把描述文本附加到聊天输入中，帮助 AI 理解图片内容。

### 3. YouTube Music 下载

启用后，bot 会识别消息中的 YouTube Music 链接并尝试下载音频。

### 4. 语录与互动

项目包含群互动与语录相关能力，主要代码在 `internal/handler/quotation`。

### 5. 反馈

用户可以通过 `/feedback <内容>` 提交反馈。

bot 会将反馈分两条消息发送给管理员：
- 第一条是带分隔符的来源信息
- 第二条是用户反馈原文，便于管理员直接复制

## 运行方式

### 环境要求

- Go 1.24.5 或更高
- 一个 Telegram Bot Token
- 可被 Telegram 访问到的 HTTPS webhook 地址
- 已配置的 AI 服务

### 1. 准备配置

复制配置文件：

```bash
cp "config copy.toml" config.toml
```

按需填写：
- `webHookConfig.token`
- `webHookConfig.address`
- `webHookConfig.domain`
- `webHookConfig.secret`
- `ai.*`
- `storage.*`

如果不想把 token 写进文件，也可以通过环境变量提供：
- `TOKEN`
- `WEBHOOK_ADDRESS`
- `WEBHOOK_DOMAIN`
- `WEBHOOK_SECRET`

### 2. 构建

```bash
make
```

或构建所有可执行程序：

```bash
make all
```

### 3. 启动

```bash
go run ./cmd/bot -config ./config.toml
```

## 配置说明

### `webHookConfig`

- `token`: Telegram bot token
- `address`: 本地 webhook 监听地址，例如 `:8080`
- `domain`: Telegram 可访问到的 HTTPS 域名
- `secret`: webhook secret，可留空自动生成
- `certFile` / `keyFile`: 直连 HTTPS 时使用

### `ai`

- `enable`: 是否启用聊天能力
- `openaiKey`: OpenAI 或兼容接口的 API Key
- `openaiModel`: 聊天模型名
- `openaiBaseUrl`: 兼容接口地址
- `fallbackOpenaiKey` / `fallbackOpenaiModel` / `fallbackOpenaiBaseUrl`: 付费兜底模型配置；免费模型额度或限流时自动切换
- `geminiKey`: 用于图片理解
- `geminiModel`: Gemini 模型名

说明：
- 当前聊天主链路会初始化 OpenAI client
- 如果配置了 `geminiKey`，图片会额外走 Gemini 做描述
- 随机插话基础概率是 0.003，每个群可在 `./data/group_reply_trigger.json` 里按 chat id 配置 0-10 倍率；0 表示该群绝不随机插话，10 表示概率放大 10 倍
- 每个群的表情 NSFW 模式保存在 `./data/group_emotion_nsfw.json`；默认值是 `0`，也就是只搜正常图

`./data/group_reply_trigger.json` 示例：

```json
{
  "default": 1,
  "groups": {
    "-1001234567890": 0,
    "-1009876543210": 10
  }
}
```

也可以只写群 ID 到倍率的简单映射：

```json
{
  "-1001234567890": 0,
  "-1009876543210": 10
}
```

### `emotion`

- `enable`: 是否启用群聊贴纸采集上传
- `apiBaseUrl`: emotion-palette-service 地址，默认 `https://emo.whosworld.fun`
- `apiKey`: 批量上传接口的 `x-api-key`
- `uploadedFile`: 本地去重记录文件路径

### `admin`

- `chatIDs`: 接收用户反馈的管理员 chat id 列表

### `storage`

支持：
- `sqlite`
- `mysql`

聊天记录会持久化到数据库中，用于恢复部分上下文。

## 项目结构

```text
cmd/
  bot/                Telegram bot 入口
internal/
  app/                应用装配
  ai/                 AI provider 抽象与实现
  bot/                Telegram webhook / polling 接入
  handler/            消息处理逻辑
  scheduler/          定时任务
  storage/            数据库与 repo
pkg/
  config/             配置加载
  logger/             日志初始化
  util/               下载等通用工具
```

## 聊天链路

主要流程：

1. `cmd/bot/main.go` 读取配置并启动应用
2. `internal/app/app.go` 装配 handler、存储、调度器和 webhook
3. `internal/handler/chat.go` 判断消息是否触发聊天
4. 触发后拼接群聊上下文、图片描述和用户输入
5. 调用 AI provider
6. 将 AI 回复拆分后发送回 Telegram

## 已知事实

- README 以当前代码为准，不再沿用旧描述
- 默认启动路径是 webhook，不是 polling
- 群聊上下文目前是内存短缓存，不是完整会话编排
- AI 行为风格主要由 `internal/handler/chat.go` 中的 prompt 控制

## 开发建议

如果后续主要维护 `chat`，优先关注这些位置：
- `internal/handler/chat.go`
- `internal/handler/cache.go`
- `internal/handler/update/update.go`
- `internal/ai/openai/client.go`
- `internal/app/app.go`

## 备注

初始化维护说明见 `docs/INIT.md`。
