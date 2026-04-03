# Project Init

这份文档记录我接手该项目后的第一版维护基线。

## 项目定位

这是一个 Telegram 群聊机器人项目，当前第一优先级不是下载、语录或调度，而是 `chat`:
- 在群里接住上下文
- 用 AI 生成自然回复
- 维持稳定、可控、可调试的对话体验

## 关键路径

核心文件：
- `cmd/bot/main.go`
- `internal/app/app.go`
- `internal/handler/chat.go`
- `internal/handler/cache.go`
- `internal/ai/openai/client.go`
- `internal/handler/update/update.go`

## 当前理解

### 触发

聊天会在以下条件触发：
- 私聊 bot
- `/chat ...`
- 回复 bot
- `@bot`
- 小概率随机触发

### 上下文

- 群聊普通消息先写入内存缓存
- 触发聊天时再把缓存拼成“对话历史”
- 数据库中保留长期聊天记录，OpenAI client 启动时会尝试恢复一部分历史

### AI 结构

- 代码抽象支持多 provider
- 当前聊天主链路实际走 OpenAI client
- Gemini 主要用于图片理解辅助

## 近期优先级

1. 保证聊天触发规则稳定，避免误判或与其他 handler 冲突
2. 提升群聊上下文质量，减少答非所问
3. 让 README、配置样例和代码行为保持一致
4. 为 chat 加上更明确的观测点和测试面

## 已发现的维护点

- 历史文档与当前代码存在偏差
- 配置样例此前包含敏感 token，不适合继续保留
- chat 的 prompt 直接写在 handler 中，后续如果继续扩展，建议抽离
- 群聊缓存是进程内状态，重启后会丢失

## 接手原则

- 后续修改优先围绕 `chat` 做，不会先做外围功能美化
- 文档以代码实际行为为准
- 涉及回复风格、触发规则、上下文拼接的改动，会优先考虑可控性和可回归验证
