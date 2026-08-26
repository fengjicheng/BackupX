---
sidebar_position: 2
title: Contributing
description: How to report issues, propose changes, and submit PRs.
---

# Contributing

BackupX is open-source under Apache License 2.0. Issues and pull requests are welcome.

## Reporting bugs

Open an issue at [github.com/Awuqing/BackupX/issues](https://github.com/Awuqing/BackupX/issues). Please include:

- BackupX version (`backupx --version`)
- Your deployment mode (Docker / bare metal / from source)
- Relevant backup task type and storage backend
- Steps to reproduce
- Stdout / `backupx.log` excerpt for the window around the problem

## Proposing changes

For significant features or refactors, open an issue first to align on scope before investing in a PR.

## Pull requests

1. Fork and create a topic branch (e.g. `fix/windows-path-escape`)
2. Run `make verify` and make sure formatting, tests, builds, and documentation checks pass
3. Keep changes focused — one concern per PR
4. Write Conventional Commits with a Chinese subject — examples:
   - `feat(audit): 新增审计日志模块`
   - `fix(browser): 修复目录浏览器无法进入子目录`
   - `refactor(storage): 简化存储目标解密逻辑`
   - Types: `feat` / `fix` / `docs` / `style` / `refactor` / `perf` / `test` / `chore`
5. PR title and body in Chinese too. Describe the why and how, not just the what.

## Coding guidelines

- **Go** — handle every error (no `_ = err`); use the existing logger (`zap`); no `fmt.Println` in production paths
- **TypeScript** — strict mode, no implicit any, and pass the repository ESLint and Prettier checks
- **Commit scope** — one logical change per commit; don't mix drive-by cleanups with feature work
