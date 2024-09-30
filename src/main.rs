mod error;
mod oss;
mod ssh;

use clap::Parser;
use error::TransferError;
use oss::{handle_oss, parse_destiontion_oss};
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
    destination: String,

    /// Transfer files to aliyun OSS
    /// base64 encoded Configuration
    #[clap(short, long, default_value = "{}")]
    oss_destionation: String,

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
    if let Ok(ssh_config) = parse_destination_ssh(&args.destination) {
        return Ok(handle_ssh(&args, ssh_config)?);
    } else {
        if let Ok(oss_config) = parse_destiontion_oss(&args.oss_destionation) {
            return Ok(handle_oss(&args.source, oss_config)?)
        } else {
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
    }
}
