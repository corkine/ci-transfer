mod error;
mod oss;
mod ssh;

use clap::Parser;
use error::TransferError;
use oss::{handle_oss, parse_destination_oss};
use ssh::{handle_ssh, parse_destination_ssh};

#[derive(Parser, Debug)]
#[clap(author, version, about, long_about = None)]
struct Args {
    /// Source file or directory path
    #[clap(short, long)]
    source: String,

    /// Destination in format user:pass@ip:/path
    /// Or base64 encoded destination
    #[clap(short, long)]
    destination: Option<String>,

    /// Transfer files to aliyun OSS
    /// base64 encoded Configuration
    #[clap(short, long)]
    oss_destination: Option<String>,

    /// SSH commands to execute before transfer
    /// Or base64 encoded commands
    #[clap(long, multiple_values = true)]
    precommands: Vec<String>,

    /// SSH commands to execute after transfer
    /// Or base64 encoded commands
    #[clap(short, long, multiple_values = true)]
    commands: Vec<String>,

    /// SSH port (default: 22)
    #[clap(long, default_value = "22")]
    port: u16,
}

fn main() -> Result<(), TransferError> {
    let args = Args::parse();
    let mut transfer_done = false;
    let mut errors: Vec<String> = Vec::new();

    if let Some(oss_dest) = &args.oss_destination {
        transfer_done = true;
        match parse_destination_oss(oss_dest) {
            Ok(oss_config) => {
                if let Err(e) = handle_oss(&args.source, oss_config) {
                    errors.push(format!("OSS transfer failed: {}", e));
                }
            }
            Err(_) => {
                errors.push("Invalid oss_destionation format".to_string());
            }
        }
    }

    if let Some(destination) = &args.destination {
        transfer_done = true;
        match parse_destination_ssh(destination) {
            Ok(ssh_config) => {
                if let Err(e) = handle_ssh(&args, ssh_config) {
                    errors.push(format!("SSH transfer failed: {}", e));
                }
            }
            Err(_) => {
                errors.push("Invalid destination format".to_string());
            }
        }
    }

    if !transfer_done {
        let json_str = r#"
        {
            "oss_bucket": "my-bucket",
            "oss_endpoint": "oss-cn-beijing.aliyuncs.com",
            "key_secret": "your-secret-key",
            "key_id": "your-access-key-id",
            "path": "/path/oss",
            "override_existing": true
        }
        "#;
        return Err(TransferError::Other(
            format!(
                "Destination cannot be empty,
            you can put user:pass@ip:/path to use ssh destionation, 
            or put json format like {json_str} to use aliyun oss destination
            or use base64 encode ssh/oss format"
            )
            .into(),
        ));
    }

    if !errors.is_empty() {
        return Err(TransferError::Other(errors.join("\n").into()));
    }

    Ok(())
}
