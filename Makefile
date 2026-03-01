# 变量定义
BUILD_DIR := bin
CMD_DIR := cmd
# 自动获取 cmd/ 下的所有子目录名 (例如 bot, api 等)
SERVICES := $(shell ls $(CMD_DIR))

.PHONY: all clean $(SERVICES) help

# 默认目标：先清理，再构建 cmd/bot
default: clean bot

# all: 先清理，再构建 cmd/ 下的所有程序
all: clean $(SERVICES)

# 动态构建规则
$(SERVICES):
	@echo "==> 构建模块: $@"
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$@ ./$(CMD_DIR)/$@

# clean: 清理 bin 目录
clean:
	@echo "==> 清理构建产物..."
	@rm -rf $(BUILD_DIR)

help:
	@echo "可用命令:"
	@echo "  make         - 默认先清理并构建 bot"
	@echo "  make all     - 清理并构建 cmd/ 下的所有程序"
	@echo "  make <name>  - 构建指定的程序 (例如: make bot)"
	@echo "  make clean   - 删除 bin 目录"
