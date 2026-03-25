# Tasks

## 1. Bootstrap Script And Skill Entry

- [x] 1.1 Add `skills/fund-manager/scripts/bootstrap-fund-manager.sh` with OS/arch detection and target triple mapping.
- [x] 1.2 Implement binary download, extraction, cache install, and executable permission handling in bootstrap script.
- [x] 1.3 Add integrity verification and redownload fallback for corrupted local cache.
- [x] 1.4 Add `skills/fund-manager/scripts/fund-manager.sh` as a stable wrapper that invokes bootstrap and forwards CLI arguments.
- [x] 1.5 Add environment-variable controls for version pinning (`FUND_MANAGER_VERSION`) and source override (`FUND_MANAGER_CORE_URL`).

## 2. GitLab CI Cross-Platform Artifacts

- [x] 2.1 Add `.gitlab-ci.yml` jobs for Linux, macOS, and Windows release builds.
- [x] 2.2 Ensure CI jobs package outputs using version + target triple naming convention.
- [x] 2.3 Configure tag-triggered publish flow to long-lived download endpoints (Package Registry or Release assets).
- [x] 2.4 Validate produced artifacts can be fetched by bootstrap script URLs for all supported targets.

## 3. Skill Documentation And Validation

- [x] 3.1 Update `skills/fund-manager/SKILL.md` to document runtime bootstrap flow and script-based entrypoint usage.
- [x] 3.2 Add `openclaw.requires` metadata for required binaries and authentication environment variables.
- [x] 3.3 Document private GitLab access configuration and troubleshooting for 401/403 download failures.
- [x] 3.4 Run end-to-end checks on at least one platform for first-run download and cached rerun behavior.
