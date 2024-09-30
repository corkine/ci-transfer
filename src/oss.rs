use std::{
    fmt::format,
    fs::{self, File},
    path::Path,
};

use crate::{error::TransferError, Args};
use aliyun_oss_rust_sdk::oss::OSS;
use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct OssConfig {
    oss_bucket: String,
    oss_endpoint: String,
    key_secret: String,
    key_id: String,
    destionation: String,
    #[serde(default)] // 使用默认值，如果 JSON 中没有这个字段
    override_existing: Option<bool>,
}

impl From<OssConfig> for OSS {
    fn from(value: OssConfig) -> Self {
        OSS::new(
            value.oss_bucket,
            value.key_id,
            value.key_secret,
            value.oss_endpoint.clone(),
        )
    }
}

pub fn parse_destiontion_oss(destination: &str) -> Result<OssConfig, TransferError> {
    if destination.is_empty() {
        return Err(TransferError::Other("Destination cannot be empty".into()));
    }
    match base64::decode(&destination) {
        Ok(decoded) => match std::str::from_utf8(&decoded) {
            Ok(s) => return parse_destiontion_oss(s),
            _ => (),
        },
        _ => (),
    }
    let config: OssConfig =
        serde_json::from_str(destination).map_err(|e| TransferError::JsonParseError(e))?;
    Ok(config)
}

fn get_files(path: &str) -> Result<Vec<String>, TransferError> {
    let path = Path::new(path);

    if !path.exists() {
        return Err(TransferError::Other("Path does not exist".into()));
    }

    if path.is_dir() {
        let mut files = Vec::new();
        collect_files_recursive(path, &mut files)?;
        Ok(files)
    } else if path.is_file() {
        Ok(vec![path.to_string_lossy().into_owned()])
    } else {
        Err(TransferError::Other(
            "Path is neither a file nor directory".into(),
        ))
    }
}

fn collect_files_recursive(dir: &Path, files: &mut Vec<String>) -> Result<(), TransferError> {
    if dir.is_dir() {
        for entry in fs::read_dir(dir)? {
            let entry = entry?;
            let path = entry.path();
            if path.is_dir() {
                collect_files_recursive(&path, files)?;
            } else {
                files.push(path.to_string_lossy().into_owned());
            }
        }
    }
    Ok(())
}

pub fn handle_oss(source: &str, oss_config: OssConfig) -> Result<(), TransferError> {
    use aliyun_oss_rust_sdk::request::RequestBuilder;

    let oss: OSS = oss_config.clone().into();
    let build = RequestBuilder::new();

    let files = get_files(source)?;
    for file in files {
        let path = Path::new(&file);
        let path = path.strip_prefix(source).unwrap();
        let real_path = Path::new(&oss_config.destionation)
            .join(path.to_string_lossy().into_owned())
            .to_string_lossy()
            .into_owned()
            .replace("\\", "/");
        oss.put_object_from_file(real_path, file, build.clone())
        .map_err(|e| TransferError::OssError(format!("{}", e)))?;
    }

    Ok(())
}

#[test]
fn test_get_files() {
    let files = get_files("src").unwrap();
    dbg!(files);
}

#[test]
fn test_handle_oss() {
    let _ = handle_oss("src", OssConfig {
        destionation: "/test".into(),
        oss_bucket: "test".into(),
        oss_endpoint: "http://oss-cn-hangzhou.aliyuncs.com".into(),
        key_id: "test".into(),
        key_secret: "test".into(),
        override_existing: None
    });
}