use clap::Parser;
use ssh2::Session;
use std::borrow::Cow;
use std::fs::{read_dir, File};
use std::io::{Read, Write};
use std::net::TcpStream;
use std::path::Path;
use std::time::{Duration, Instant};

#[derive(Parser, Debug)]
#[clap(author, version, about, long_about = None)]
struct Args {
    /// Source file or directory path
    #[clap(short, long)]
    source: String,

    /// Destination in format user:pass@ip:/path
    #[clap(short, long)]
    destination: String,

    /// SSH commands to execute before transfer
    #[clap(long, multiple_values = true)]
    precommands: Vec<String>,

    /// SSH commands to execute after transfer
    #[clap(short, long, multiple_values = true)]
    commands: Vec<String>,

    /// SSH port (default: 22)
    #[clap(long, default_value = "22")]
    port: u16,
}

struct SshConfig {
    username: String,
    password: String,
    ip: String,
    remote_path: String,
}

#[derive(Debug)]
enum TransferError {
    IoError(std::io::Error),
    SshError(ssh2::Error),
    Other(String),
}

impl From<std::io::Error> for TransferError {
    fn from(error: std::io::Error) -> Self {
        TransferError::IoError(error)
    }
}

impl From<ssh2::Error> for TransferError {
    fn from(error: ssh2::Error) -> Self {
        TransferError::SshError(error)
    }
}

impl std::fmt::Display for TransferError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            TransferError::IoError(e) => write!(f, "IO Error: {}", e),
            TransferError::SshError(e) => write!(f, "SSH Error: {}", e),
            TransferError::Other(s) => write!(f, "Other Error: {}", s),
        }
    }
}

fn parse_destination(destination: &str) -> Result<SshConfig, TransferError> {
    if destination.is_empty() {
        return Err(TransferError::Other("Destination cannot be empty".into()));
    }
    match base64::decode(&destination) {
        Ok(decoded) => match std::str::from_utf8(&decoded) {
            Ok(s) => return parse_destination(s),
            _ => (),
        },
        _ => (),
    }
    let parts: Vec<&str> = destination.split('@').collect();
    if parts.len() != 2 {
        return Err(TransferError::Other("Invalid destination format".into()));
    }

    let credentials: Vec<&str> = parts[0].split(':').collect();
    if credentials.len() != 2 {
        return Err(TransferError::Other("Invalid credentials format".into()));
    }

    let server_info: Vec<&str> = parts[1].split(':').collect();
    if server_info.len() != 2 {
        return Err(TransferError::Other("Invalid server info format".into()));
    }

    Ok(SshConfig {
        username: credentials[0].to_string(),
        password: credentials[1].to_string(),
        ip: server_info[0].to_string(),
        remote_path: server_info[1].to_string(),
    })
}

fn transfer_file(
    session: &Session,
    local_path: &Path,
    remote_path: &str,
) -> Result<(), TransferError> {
    let mut local_file = File::open(local_path)?;
    let file_size = local_file.metadata()?.len();
    let mut remote_file = session.scp_send(Path::new(remote_path), 0o644, file_size, None)?;

    let mut buffer = vec![0; 1024 * 1024]; // 1MB buffer
    let mut total_transferred = 0;
    let start_time = Instant::now();
    let mut last_update = Instant::now();

    loop {
        let bytes_read = local_file.read(&mut buffer)?;
        if bytes_read == 0 {
            break;
        }
        remote_file.write_all(&buffer[..bytes_read])?;
        total_transferred += bytes_read as u64;

        // Update progress every second
        if last_update.elapsed() >= Duration::from_secs(1) {
            print_progress(total_transferred, file_size, start_time.elapsed());
            last_update = Instant::now();
        }
    }

    remote_file.send_eof()?;
    remote_file.wait_eof()?;
    remote_file.close()?;
    remote_file.wait_close()?;

    print_progress(total_transferred, file_size, start_time.elapsed());
    println!("\nTransferred: {:?} -> {}", local_path, remote_path);
    Ok(())
}

fn print_progress(transferred: u64, total: u64, elapsed: Duration) {
    let percentage = (transferred as f64 / total as f64) * 100.0;
    let speed = transferred as f64 / elapsed.as_secs_f64() / 1024.0 / 1024.0; // MB/s
    print!(
        "\rProgress: {:.2}% ({}/{} bytes) - {:.2} MB/s",
        percentage, transferred, total, speed
    );
    std::io::stdout().flush().unwrap();
}

fn transfer_directory(
    session: &Session,
    local_dir: &Path,
    remote_dir: &str,
) -> Result<(), TransferError> {
    let sftp = session.sftp()?;
    sftp.mkdir(Path::new(remote_dir), 0o755)?;

    for entry in read_dir(local_dir)? {
        let entry = entry?;
        let path = entry.path();
        let file_name = path.file_name().unwrap().to_str().unwrap();
        let remote_path = format!("{}/{}", remote_dir, file_name);

        if path.is_dir() {
            transfer_directory(session, &path, &remote_path)?;
        } else {
            transfer_file(session, &path, &remote_path)?;
        }
    }

    Ok(())
}

fn transfer(session: &Session, source: &str, remote_path: &str) -> Result<(), TransferError> {
    println!("Transferring: {} -> {}", source, remote_path);
    let source_path = Path::new(source);
    if source_path.is_dir() {
        transfer_directory(session, source_path, remote_path)
    } else {
        let remote_file_path = if remote_path.ends_with('/') {
            format!(
                "{}{}",
                remote_path,
                source_path.file_name().unwrap().to_str().unwrap()
            )
        } else {
            remote_path.to_string()
        };
        transfer_file(session, source_path, &remote_file_path)
    }
}

fn execute_ssh_commands(session: &Session, commands: &[String]) -> Result<(), TransferError> {
    for command in commands {
        if command.is_empty() {
            continue;
        }
        if let Ok(decoded) = base64::decode(&command) {
            if let Ok(decoded_str) = std::str::from_utf8(&decoded) {
                execute_ssh_commands(session, &[decoded_str.to_string()])?;
                continue;
            }
        }
        let mut channel = session.channel_session()?;
        let escaped_command = escape_command(command);
        let wrapped_command = format!("bash -c {}", escaped_command);
        channel.exec(&wrapped_command)?;
        let mut output = String::new();
        channel.read_to_string(&mut output)?;
        println!("Command: {}", command);
        println!("Output: {}", output);
        channel.wait_close()?;
        println!("Exit status: {}", channel.exit_status()?);
        println!("---");
    }
    Ok(())
}

fn escape_command(cmd: &str) -> Cow<str> {
    if cmd.contains('"') || cmd.contains('\\') {
        let escaped = cmd.replace('"', "\\\"").replace('\\', "\\\\");
        Cow::Owned(format!("\"{}\"", escaped))
    } else {
        Cow::Borrowed(cmd)
    }
}

fn main() -> Result<(), TransferError> {
    let args = Args::parse();
    let ssh_config = parse_destination(&args.destination)?;

    let tcp = TcpStream::connect(format!("{}:{}", ssh_config.ip, args.port))?;
    println!("Connected to {}:{}", ssh_config.ip, args.port);
    let mut session = Session::new()?;
    session.set_timeout(0);
    session.set_tcp_stream(tcp);
    session.handshake()?;
    session.userauth_password(&ssh_config.username, &ssh_config.password)?;

    // Execute precommands if they exist
    if !args.precommands.is_empty() {
        println!("Executing pre-transfer commands:");
        execute_ssh_commands(&session, &args.precommands)?;
        println!("Pre-transfer commands completed.");
    }

    transfer(&session, &args.source, &ssh_config.remote_path)?;
    println!("\nFile(s) transferred successfully");

    if !args.commands.is_empty() {
        println!("Executing post-transfer commands:");
        execute_ssh_commands(&session, &args.commands)?;
        println!("Post-transfer commands completed.");
    }

    Ok(())
}
