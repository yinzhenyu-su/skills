use dirs;
use std::env;
use std::path::PathBuf;

use serde::{Deserialize, Serialize};
use std::fs;
use std::io;

pub const DEFAULT_USER_AGENT: &str = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36";

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct Config {
    #[serde(default)]
    pub default_market_items: Vec<String>,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            default_market_items: Vec::new(),
        }
    }
}

impl Config {
    pub fn load() -> Self {
        let path = get_config_path();
        if !path.exists() {
            return Self::default();
        }

        let content = fs::read_to_string(path).unwrap_or_default();
        serde_json::from_str(&content).unwrap_or_else(|_| Self::default())
    }

    pub fn save(&self) -> io::Result<()> {
        let path = get_config_path();
        let app_dir = get_app_dir();
        if !app_dir.exists() {
            fs::create_dir_all(app_dir)?;
        }
        let content = serde_json::to_string_pretty(self).map_err(|e| io::Error::new(io::ErrorKind::Other, e))?;
        fs::write(path, content)
    }
}

pub fn get_user_agent() -> String {
    env::var("FUND_MANAGER_UA").unwrap_or_else(|_| DEFAULT_USER_AGENT.to_string())
}

pub fn get_app_dir() -> PathBuf {
    if let Ok(path) = env::var("FUND_MANAGER_APP_DIR") {
        return PathBuf::from(path);
    }

    #[cfg(unix)]
    {
        let mut path = dirs::home_dir().expect("Could not find home directory");
        path.push(".config");
        path.push("fund-manager");
        path
    }

    #[cfg(windows)]
    {
        let mut path = dirs::config_dir().expect("Could not find config directory");
        path.push("fund-manager");
        path
    }

    #[cfg(not(any(unix, windows)))]
    {
        let mut path = dirs::config_dir().expect("Could not find config directory");
        path.push("fund-manager");
        path
    }
}

pub fn get_db_path() -> PathBuf {
    let mut path = get_app_dir();
    path.push("fund.db");
    path
}

pub fn get_config_path() -> PathBuf {
    let mut path = get_app_dir();
    path.push("config.json");
    path
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_paths_and_env_override() {
        // 1. Test default path logic
        // We must ensure the env var is not set
        unsafe {
            env::remove_var("FUND_MANAGER_APP_DIR");
        }

        let app_dir = get_app_dir();
        println!("Default App dir: {:?}", app_dir);

        #[cfg(unix)]
        {
            let path_str = app_dir.to_str().unwrap();
            assert!(path_str.contains(".config"));
            assert!(path_str.contains("fund-manager"));
        }

        #[cfg(windows)]
        {
            assert!(app_dir.to_str().unwrap().contains("fund-manager"));
        }

        let db_path = get_db_path();
        assert!(db_path.to_str().unwrap().ends_with("fund.db"));

        // 2. Test env override
        let test_path = "/tmp/my-fund-manager-override";
        unsafe {
            env::set_var("FUND_MANAGER_APP_DIR", test_path);
        }
        let app_dir_overridden = get_app_dir();
        assert_eq!(app_dir_overridden.to_str().unwrap(), test_path);
        unsafe {
            env::remove_var("FUND_MANAGER_APP_DIR");
        }
    }
}
