use assert_cmd::Command;
use std::path::PathBuf;
use tempfile::{TempDir, tempdir_in};
use std::fs;

pub struct TestContext {
    #[allow(dead_code)]
    temp_dir: TempDir,
    app_dir: PathBuf,
}

impl TestContext {
    pub fn new(name: &str) -> Self {
        // Create a base directory for tests in target/
        let target_tests_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("target")
            .join("tests");
        
        if !target_tests_dir.exists() {
            fs::create_dir_all(&target_tests_dir).unwrap();
        }

        // Create a unique temporary directory within target/tests/
        let temp_dir = tempdir_in(target_tests_dir).expect("Failed to create temp dir");
        let app_dir = temp_dir.path().join(name);
        fs::create_dir_all(&app_dir).unwrap();

        Self {
            temp_dir,
            app_dir,
        }
    }

    pub fn app_dir(&self) -> &PathBuf {
        &self.app_dir
    }

    pub fn cmd(&self) -> Command {
        let mut cmd = Command::cargo_bin("fund-manager").unwrap();
        cmd.env("FUND_MANAGER_APP_DIR", self.app_dir.to_str().unwrap());
        cmd
    }
}
