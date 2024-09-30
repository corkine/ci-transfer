# ci-transfer

在 Github Action 中传输文件到虚拟机并执行命令：

```bash
./your_program_name --source ./large_file.zip --destination user:password@192.168.1.100:/remote/path/ --port 2222 --precommands "df -h" --commands "echo 'Transfer complete!'"
```

在 Github Action 中如何使用？首先创建仓库 Secret，然后使用最新的 `ci-transfer` 将二进制传输并部署到远程服务器。

```yaml
- name: Run ci-transfer
  env:
    DESTINATION: ${{ secrets.DESTINATION }}
  run: |
    wget https://github.com/corkine/ci-transfer/releases/latest/download/ci-transfer
    chmod +x ci-transfer
    ./ci-transfer -s target/x86_64-unknown-linux-musl/release/calibre-api -d "$DESTINATION" --precommands "rm -f /root/calibre-web/calibre-api" -c "/root/calibre-web/deploy.sh"
```

可使用 --oss_destination 参数将文件上传至 OSS，传入的内容为 JSON 格式字符串或 Base64 编码字符串：

```json
{
    "oss_bucket": "my-bucket",
    "oss_endpoint": "oss-cn-beijing.aliyuncs.com",
    "key_secret": "your-secret-key",
    "key_id": "your-access-key-id",
    "path": "/path/oss",
    "override_existing": true
}
```