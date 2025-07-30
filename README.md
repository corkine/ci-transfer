# ci-transfer

在 Github Action 中传输文件到虚拟机并执行命令的工具。

## 功能特性

- ✅ **单文件传输**：支持传输单个文件
- ✅ **文件夹传输**：支持递归传输整个文件夹
- 🆕 **多文件/文件夹传输**：支持同时传输多个文件和文件夹
- 🆕 **纯副作用操作**：支持不传输文件，仅执行预命令和后置命令
- ✅ **SSH传输**：通过SSH协议传输到远程服务器
- ✅ **OSS上传**：支持上传到阿里云对象存储
- ✅ **命令执行**：支持传输前后执行自定义命令

## 基本用法

### 单文件传输
```bash
./ci-transfer --source ./large_file.zip --destination user:password@192.168.1.100:/remote/path/ --port 2222 --precommands "df -h" --commands "echo 'Transfer complete!'"
```

### 多文件/文件夹传输
```bash
# 传输多个文件和文件夹
./ci-transfer -s file1.txt -s file2.txt -s folder/ -d user:pass@192.168.1.100:/upload/

# 或使用长参数格式
./ci-transfer --source app/ --source config.json --source data/ --destination user:pass@server:/deploy/
```

### 纯副作用操作（不传输文件）
```bash
# 只执行命令，不传输任何文件
./ci-transfer -d user:pass@server:/tmp/ --precommands "mkdir -p /backup" --commands "systemctl restart myapp"
```

## 在 GitHub Actions 中使用

首先创建仓库 Secret，然后使用最新的 `ci-transfer` 将文件传输并部署到远程服务器。

### 单文件部署示例
```yaml
- name: Deploy single file
  env:
    DESTINATION: ${{ secrets.DESTINATION }}
  run: |
    wget https://github.com/corkine/ci-transfer/releases/latest/download/ci-transfer
    chmod +x ci-transfer
    ./ci-transfer -s target/x86_64-unknown-linux-musl/release/calibre-api -d "$DESTINATION" --precommands "rm -f /root/calibre-web/calibre-api" -c "/root/calibre-web/deploy.sh"
```

### 多文件部署示例
```yaml
- name: Deploy multiple files
  env:
    DESTINATION: ${{ secrets.DESTINATION }}
  run: |
    wget https://github.com/corkine/ci-transfer/releases/latest/download/ci-transfer
    chmod +x ci-transfer
    ./ci-transfer -s dist/ -s config.yml -s assets/ -d "$DESTINATION" --precommands "systemctl stop myapp" --commands "systemctl start myapp"
```

### 纯命令执行示例
```yaml
- name: Execute remote commands only
  env:
    DESTINATION: ${{ secrets.DESTINATION }}
  run: |
    wget https://github.com/corkine/ci-transfer/releases/latest/download/ci-transfer
    chmod +x ci-transfer
    ./ci-transfer -d "$DESTINATION" --precommands "docker pull myapp:latest" --commands "docker-compose restart"
```

## OSS 上传功能

可使用 `--oss-destination` 参数将文件上传至阿里云 OSS，传入的内容为 JSON 格式字符串或 Base64 编码字符串。

### 路径规则
- **单文件**：如果 `path` 以 `/` 结尾，则上传到此目录；否则将其看做上传到的文件位置
- **文件夹**：递归上传文件夹内所有文件到 `path` 下，保持目录结构
- **多文件/文件夹**：自动为每个文件/文件夹创建唯一路径，避免冲突
- **纯副作用**：不指定 `--source` 参数时，仅验证 OSS 配置，不上传任何文件

### 使用示例
```bash
# 单文件上传
./ci-transfer -s myfile.txt --oss-destination "your-base64-config"

# 多文件上传
./ci-transfer -s file1.txt -s file2.txt -s folder/ --oss-destination "your-base64-config"

# 仅验证配置
./ci-transfer --oss-destination "your-base64-config"
```

### OSS 配置格式
```json
{
    "oss_bucket": "my-bucket",
    "oss_endpoint": "oss-cn-beijing.aliyuncs.com",
    "key_secret": "your-secret-key",
    "key_id": "your-access-key-id",
    "path": "/path/to/oss/",
    "override_existing": true
}
```

## 参数说明

| 参数 | 短参数 | 描述 | 示例 |
|------|-------|------|------|
| `--source` | `-s` | 源文件或文件夹路径（可多个，可为空） | `-s file1.txt -s folder/` |
| `--destination` | `-d` | SSH目标格式：`user:pass@ip:/path` | `-d user:pass@192.168.1.100:/upload/` |
| `--oss-destination` | 无 | OSS配置（JSON或Base64编码） | `--oss-destination "your-config"` |
| `--precommands` | 无 | 传输前执行的命令（可多个） | `--precommands "systemctl stop app"` |
| `--commands` | `-c` | 传输后执行的命令（可多个） | `-c "systemctl start app"` |
| `--port` | 无 | SSH端口（默认22） | `--port 2222` |