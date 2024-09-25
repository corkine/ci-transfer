# cl-transfer

在 Github Action 中通过 SSH 和 SCP 传输文件并执行命令

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