# cl-transfer

在 Github Action 中通过 SSH 和 SCP 传输文件并执行命令

```bash
./your_program_name --source ./large_file.zip --destination user:password@192.168.1.100:/remote/path/ --port 2222 --precommands "df -h" --commands "echo 'Transfer complete!'"
```