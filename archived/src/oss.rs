use std::{
    fs::{self},
    path::Path,
};

use crate::error::TransferError;
use aliyun_oss_rust_sdk::oss::OSS;
use base64::{engine::general_purpose, Engine as _};
use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct OssConfig {
    oss_bucket: String,
    oss_endpoint: String,
    key_secret: String,
    key_id: String,
    path: String,
    #[serde(default)] // 使用默认值，如果 JSON 中没有这个字段
    override_existing: Option<bool>,
}

impl From<OssConfig> for OSS {
    fn from(value: OssConfig) -> Self {
        OSS::new(
            value.key_id,
            value.key_secret,
            value.oss_endpoint.clone(),
            value.oss_bucket,
        )
    }
}

pub fn parse_destination_oss(destination: &str) -> Result<OssConfig, TransferError> {
    if destination.is_empty() {
        return Err(TransferError::Other("Destination cannot be empty".into()));
    }
    match general_purpose::STANDARD.decode(&destination) {
        Ok(decoded) => match std::str::from_utf8(&decoded) {
            Ok(s) => return parse_destination_oss(s),
            _ => (),
        },
        _ => (),
    }
    let config: OssConfig =
        serde_json::from_str(destination).map_err(|e| TransferError::JsonParseError(e))?;
    Ok(config)
}

pub fn handle_oss(sources: &[String], oss_config: OssConfig) -> Result<(), TransferError> {
    use aliyun_oss_rust_sdk::request::RequestBuilder;

    let oss: OSS = oss_config.clone().into();
    let build = RequestBuilder::new().with_expire(300);
    
    // Handle empty sources case (side-effect only)
    if sources.is_empty() {
        println!("No source files specified for OSS transfer - side-effect only operation");
        return Ok(());
    }

    for source in sources {
        let source_path = Path::new(source);

        if !source_path.exists() {
            return Err(TransferError::Other(format!(
                "Source path {} does not exist",
                source
            )));
        }

        if source_path.is_dir() {
            let mut dirs_to_visit = vec![source_path.to_path_buf()];
            while let Some(dir) = dirs_to_visit.pop() {
                for entry in fs::read_dir(dir)? {
                    let entry = entry?;
                    let path = entry.path();
                    if path.is_dir() {
                        dirs_to_visit.push(path);
                    } else if path.is_file() {
                        let relative_path = path.strip_prefix(source_path).unwrap();
                        let oss_object_path = Path::new(&oss_config.path)
                            .join(relative_path)
                            .to_string_lossy()
                            .into_owned();
                        let real_path = oss_object_path.replace("\\", "/");
                        
                        println!("oss transfer: {}", real_path);

                        oss.put_object_from_file(
                            real_path,
                            path.to_string_lossy().into_owned(),
                            build.clone(),
                        )
                        .map_err(|e| TransferError::OssError(format!("{}", e)))?;
                    }
                }
            }
        } else if source_path.is_file() {
            let oss_object_path = if oss_config.path.ends_with('/') {
                let file_name = source_path.file_name().unwrap().to_str().unwrap();
                format!("{}{}", oss_config.path, file_name)
            } else {
                // For multiple files, we need to create unique paths
                if sources.len() > 1 {
                    let file_name = source_path.file_name().unwrap().to_str().unwrap();
                    format!("{}/{}", oss_config.path.trim_end_matches('/'), file_name)
                } else {
                    oss_config.path.clone()
                }
            };
            let real_path = oss_object_path.replace("\\", "/");
            println!("oss transfer: {}", real_path);
            oss.put_object_from_file(real_path, source.to_string(), build.clone())
                .map_err(|e| TransferError::OssError(format!("{}", e)))?;
        } else {
            return Err(TransferError::Other(
                format!("Path {} is neither a file nor directory", source).into(),
            ));
        }
    }

    Ok(())
}

#[test]
fn test_handle_oss() {
    let _ = handle_oss(
        &["src".to_string()],
        OssConfig {
            path: "/test".into(),
            oss_bucket: "test".into(),
            oss_endpoint: "http://oss-cn-hangzhou.aliyuncs.com".into(),
            key_id: "test".into(),
            key_secret: "test".into(),
            override_existing: None,
        },
    );
}
