# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Multi-skill release workflow infrastructure
  - `skills/VERSION` for unified version management
  - `skills/scripts/lib-skill-url.sh` shared URL template library
  - `skills/<skill>/.skill.toml` for skill-specific build matrix

## [v0.1.0] - 2026-03-26

### Features

- Initial release with fund-manager skill
- Fund holding tracking and profit/loss calculation
- NAV sync from East Money and Morningstar
- Multi-platform binary distribution (Linux, macOS, Windows)
- Bootstrap script for automatic binary download
