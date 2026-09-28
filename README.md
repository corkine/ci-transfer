# ci-transfer

用于 GitHub Actions 的 Go 命令行工具：上传文件到阿里云 OSS、通过 SSH/SCP 传输文件，以及执行远程部署命令。Linux、Windows、macOS 均可使用，无需安装 Go 或 OpenSSL 即可运行发布的二进制。

原 Rust 实现及其 README、Cargo 文件、发布工作流保存在 [`archived/`](archived/)。根目录为当前 Go 实现。

## 兼容现有 CI

现有参数、JSON/Base64 Secret 格式以及 Linux 的 `ci-transfer`、Windows 的 `ci-transfer.exe` 下载文件名保持不变。以下调用无需修改；发布新版 Release 后，`latest/download` 才会下载到 Go 版本。

### OSS 上传

```yaml
- name: Run ci-transfer
  id: transfer
  env:
    OSS_DESTINATION: ${{ secrets.OSS_DESTINATION }}
  run: |
    wget https://github.com/corkine/ci-transfer/releases/latest/download/ci-transfer
    chmod +x ci-transfer
    ./ci-transfer -s "$PACKAGE_NAME" --oss-destination "$OSS_DESTINATION"
    echo "LATEST_ZIP_NAME=$PACKAGE_NAME" >> "$GITHUB_ENV"
```

APK 上传同样适用：

```bash
./ci-transfer -s build/app/outputs/flutter-apk/app-release.apk --oss-destination "$APK_OSS_DESTINATION"
```

### 上传文件并触发 SSH 部署

```yaml
- name: Trigger remote deployment
  env:
    SSH_DESTINATION: ${{ secrets.SSH_DESTINATION }}
  run: |
    wget https://github.com/corkine/ci-transfer/releases/latest/download/ci-transfer
    chmod +x ci-transfer
    echo "CyberMe Services deployment trigger - ${{ github.sha }}" > cyberme-svc.trigger
    ./ci-transfer --source ./cyberme-svc.trigger --destination "$SSH_DESTINATION" --commands "/root/cyberme/go-service.sh"
```

Windows PowerShell：

```powershell
Invoke-WebRequest -Uri https://github.com/corkine/ci-transfer/releases/latest/download/ci-transfer.exe -OutFile ci-transfer.exe
.\ci-transfer.exe -s "$env:PACKAGE_NAME" --oss-destination "$env:OSS_DESTINATION"
```

也可仅执行命令，或一次传输多个文件/目录：

```bash
./ci-transfer -d "$SSH_DESTINATION" -c "/root/cyberme/go-service.sh"
./ci-transfer -s dist/ config.json -s assets/ -d "$SSH_DESTINATION" \
  --precommands "echo preparing" --commands "echo uploaded" "systemctl restart myapp"
```

## 默认重试与配置

无需修改现有调用即可启用重试：首次失败后最多重试 **3 次**（最多 **4 次尝试**），等待时间为 **2、4、8 秒**。初始间隔可配置，每次翻倍，最大 30 秒。

```bash
# 最多重试 5 次，初始间隔 5 秒，每次文件上传最多 20 分钟
./ci-transfer -s "$PACKAGE_NAME" --oss-destination "$OSS_DESTINATION" \
  --retries 5 --retry-delay 5s --timeout 20m

# 关闭重试
./ci-transfer -s cyberme-svc.trigger -d "$SSH_DESTINATION" \
  --commands "/root/cyberme/go-service.sh" --retries 0
```

- OSS：对临时网络错误、超时、HTTP 408/429/5xx 重试。每次重新打开文件并从头上传；SDK 内层重试已关闭，不会叠加尝试次数。当前使用单请求上传，不支持分片续传。
- SSH：连接建立和文件传输遇到临时网络错误时重试。传输失败后重新连接，只重传当前文件；创建目录也支持重试。
- 鉴权失败、权限不足、配置错误、本地文件错误不重试。
- **`--precommands` 与 `--commands` 永不自动重试**。连接中断时命令可能已在远端执行，因此返回失败，不重新发送命令。上传重试不会重跑预命令。
- 每个文件的重试预算独立。`--timeout` 针对每次上传尝试，不包含退避等待；SSH 重连另受 `--connect-timeout` 限制。日志显示字节数、耗时、尝试次数与错误。
- 这些参数控制 ci-transfer 自身的操作，不控制前面的 `wget` 或 `Invoke-WebRequest` 下载。

## 配置格式

### OSS

`--oss-destination` 接受 JSON 或其标准 Base64 编码，原有 Secret 可继续使用：

```json
{
  "oss_bucket": "cmbed",
  "oss_endpoint": "oss-cn-shenzhen.aliyuncs.com",
  "key_id": "your-access-key-id",
  "key_secret": "your-access-key-secret",
  "path": "/service/flux-boot/",
  "override_existing": true
}
```

未指定协议的 endpoint 默认使用 HTTPS；显式的 `http://`、`https://` 会保留。普通地域 endpoint 自动识别签名地域；自定义或加速 endpoint 可增加 `"oss_region": "cn-shenzhen"` 使用 V4 签名，否则保持兼容的 V1 签名。实现使用[阿里云官方 Go SDK V2](https://www.alibabacloud.com/help/en/oss/developer-reference/manual-for-go-sdk-v2/)。

`override_existing` 继续接受，但与旧版一致，不影响覆盖行为：相同对象键会被覆盖。不指定 `--source` 时只检查配置格式，不验证远端权限。

### SSH

`--destination` 接受 `user:password@host:/remote/path` 或其标准 Base64 编码；端口通过 `--port` 设置。支持密码中的 `:`、`@`，IPv6 地址使用方括号。

文件使用 SCP，目录通过 SFTP 创建；远程命令通过 `bash -c` 执行。远端需要相应的程序/子系统。命令也支持标准 Base64 编码，stdout/stderr 实时输出。

为兼容 CI 中的临时 runner，默认不要求 `known_hosts`、不读取本机 `~/.ssh/known_hosts`，也不校验 SSH 主机密钥。现有 SSH Secret 可以直接使用；仅显式传入 `--known-hosts /path/to/known_hosts` 才启用校验。已配置的主机密钥不匹配时直接失败。

## 路径规则

| 源 | OSS | SSH |
| --- | --- | --- |
| 单文件 | `path` 以 `/` 结尾时追加文件名，否则使用指定对象名 | 目标以 `/` 结尾时追加文件名，否则使用指定文件路径 |
| 单目录 | 将目录内容上传到 `path` 下，保留内部结构 | 目标以 `/` 结尾时追加源目录名，否则使用目标作为目录根 |
| 多个源 | 普通文件追加文件名；目录内容均合并到 `path` 下 | 每个源追加自身文件名或目录名 |

OSS 配置中的开头 `/` 作为路径分隔符处理，`/service/app.zip` 对应对象键 `service/app.zip`。多个 OSS 源目录中相同的相对文件名会覆盖，保持旧实现行为。目录遍历不跟随内部符号链接。

## 参数

| 参数 | 默认值 | 含义 |
| --- | --- | --- |
| `-s`, `--source` | 无 | 文件/目录，可连续传多个值，也可重复参数 |
| `-d`, `--destination` | 无 | SSH 配置，明文或 Base64 |
| `--oss-destination` | 无 | OSS 配置，JSON 或 Base64 |
| `--precommands` | 无 | SSH 传输前命令，可多个 |
| `-c`, `--commands` | 无 | SSH 传输后命令，可多个 |
| `--port` | `22` | SSH 端口 |
| `--retries` | `3` | 额外重试次数，0–100，0 关闭 |
| `--retry-delay` | `2s` | 初始退避间隔，上限 30s |
| `--timeout` | `10m` | 每次文件上传/目录操作的超时 |
| `--connect-timeout` | `30s` | 连接建立/SSH 握手超时 |
| `--command-timeout` | `0` | 每条 SSH 命令超时，0 不限时 |
| `--known-hosts` | 无 | 用于校验 SSH 主机密钥的文件 |
| `-h`, `--help` | — | 显示帮助 |
| `-V`, `--version` | — | 显示版本 |

超时与间隔使用 Go duration 格式，例如 `30s`、`10m`。除命令超时外必须大于 0。

## 迁移行为说明

- 现有 OSS/SSH 调用、Secret、传输前后顺序和下载文件名保持兼容。
- **远程命令非零退出码现在会导致工具失败**，停止后续 SSH 步骤；旧 Rust 实现仅打印退出码。
- 命令使用完整 shell 引号，正确保留空格、引号、管道和变量表达式。
- 不提供 SSH 目标却指定 SSH 命令时直接报错，避免命令被静默忽略。
- Linux 发布文件固定为 amd64；`ci-transfer.macos` 为 Apple Silicon arm64，Intel Mac 使用新增的 `ci-transfer.macos-amd64`。

## 构建与测试

项目采用轻量分层：

```text
cmd/ci-transfer/     CLI 入口、进程退出码与中断处理
internal/cli/        参数解析与帮助信息
internal/transfer/   OSS、SSH、配置、路径规划与重试；测试与源码放在一起
archived/           原 Rust 实现与文档
.github/workflows/  Go 测试、跨平台构建与发布
```

需要 Go 1.26.1 或更高版本：

```bash
go test ./...
go vet ./...
go build -trimpath -o ci-transfer ./cmd/ci-transfer
```

Windows 可使用 `go build -trimpath -o ci-transfer.exe ./cmd/ci-transfer`。竞态检查使用 `go test -race ./...`，需要可用的 C 编译器。

测试使用本地 HTTP/SSH 服务注入超时、服务端错误与连接中断，验证上传重传、参数兼容和远程命令不重放，不依赖实际 OSS/SSH 账号。

GitHub Actions 在分支推送/PR 时测试并交叉编译，推送 `v*` 标签时创建 Release，包含 `ci-transfer`、`ci-transfer.exe`、`ci-transfer.macos`、`ci-transfer.macos-amd64` 和 `SHA256SUMS`。手动运行非标签版本只构建产物，不发布 Release。
