# Changelog
All notable changes to this project will be documented in this file. See [conventional commits](https://www.conventionalcommits.org/) for commit guidelines.

- - -
## v1.25.0 - 2026-10-30
#### 🚀 Features
- (**bora**) ✨ read code by sorucebot context, with history, diff and blame - (5ff318a) - *zenkiet*

- - -

## v1.24.1 - 2026-10-28
#### ♻️ Code Refactoring
- (**http**) ♻️ share image upload and download handling - (2ce1046) - *zenkiet*

- - -

## v1.24.0 - 2026-10-27
#### 🚀 Features
- (**users**) ✨ upload an avatar - (4841993) - *zenkiet*
#### ⚡ Performance Improvements
- (**metrics**) ⚡ share one stream per container and replay its last minute - (1191906) - *zenkiet*

- - -

## v1.23.1 - 2026-10-27
#### 🐛 Bug Fixes
- (**proxy**) 🐛 handle rewrite basehref - (385e757) - *zenkiet*

- - -

## v1.23.0 - 2026-10-26
#### 🚀 Features
- (**bora**) ✨ attach images to a question - (3d1f6a3) - *zenkiet*
#### 🐛 Bug Fixes
- 🐛 restart the database after a reboot - (93b0248) - *zenkiet*

- - -

## v1.22.1 - 2026-10-26
#### 🐛 Bug Fixes
- (**bora**) 🐛 read every number aloud without a panic - (3b31f4a) - *zenkiet*
- (**bora**) 🐛 keep the newest log lines - (fc8e1dd) - *zenkiet*
#### ⚡ Performance Improvements
- (**notifications**) ⚡ index each task's last deploy - (8ff4de4) - *zenkiet*
- (**projects**) ⚡ read the fleet's tasks in one query - (0c5855c) - *zenkiet*
#### 🔧 Chores & Maintenance
- (**db**) 🔖 drop the unused task_metrics table - (8810cf7) - *zenkiet*

- - -

## v1.22.0 - 2026-10-21
#### 🚀 Features
- (**web**) ✨ publish the privacy policy - (8393a81) - *zenkiet*

- - -

## v1.21.0 - 2026-10-20
#### 🚀 Features
- (**volumes**) ✨ mount task volumes writable - (3c4aeaa) - *zenkiet*

- - -

## v1.20.2 - 2026-10-20
#### 🐛 Bug Fixes
- (**proxy**) 🐛 stop use fucking injectBase - (45d3576) - *zenkiet*

- - -

## v1.20.1 - 2026-10-20
#### 🐛 Bug Fixes
- (**proxy**) 🐛 keep task HTML plain for service workers - (dc56a15) - *zenkiet*

- - -

## v1.20.0 - 2026-10-20
#### 🚀 Features
- (**proxy**) ✨ add routes on a task's App URL - (c5896df) - *zenkiet*

- - -

## v1.19.0 - 2026-10-20
#### 🚀 Features
- (**push**) ✨ send push straight to FCM HTTP v1 - (48958fe) - *zenkiet*

- - -

## v1.18.0 - 2026-10-18
#### 🚀 Features
- (**voice**) ✨ add the on-device voice plugin - (b5c6d98) - *zenkiet*

- - -

## v1.17.0 - 2026-10-12
#### 🚀 Features
- (**bora**) ✨ implement bora assistant AI - (3977cef) - *zenkiet*

- - -

## v1.16.0 - 2026-10-06
#### 🚀 Features
- (**chat**) ✨ add code chat and project repositories - (58141db) - *zenkiet*

- - -

## v1.15.0 - 2026-10-03
#### 🚀 Features
- (**events**) ✨ signal every successful write - (7f859d3) - *zenkiet*
- (**events**) ✨ stream change signals over SSE - (a496c90) - *zenkiet*

- - -

## v1.14.0 - 2026-10-02
#### 🚀 Features
- (**task**) ✨ add build progress tracking for tasks - (016d327) - *zenkiet*

- - -

## v1.13.0 - 2026-09-30
#### 🚀 Features
- ✨ implement binding volumes for task - (92e85c7) - *zenkiet*

- - -

## v1.12.0 - 2026-09-29
#### ⚡ Performance Improvements
- ⚡ improve new api - (b6ac250) - *zenkiet*

- - -

## v1.11.0 - 2026-09-17
#### 🚀 Features
- (**project**) ✨ implement note in project detail - (7a97e55) - *zenkiet*

- - -

## v1.10.2 - 2026-09-17
#### 🎨 Styles & Formatting
- 💄 improve UI/UX for project detail - (dee5e13) - *zenkiet*

- - -

## v1.10.1 - 2026-09-17
#### ♻️ Code Refactoring
- (**web**) ♻️ improve search task - (3f3351d) - *zenkiet*

- - -

## v1.10.0 - 2026-09-17
#### 🚀 Features
- (**web**) ✨ implement manage task public - (bf79fa6) - *zenkiet*

- - -

## v1.9.0 - 2026-08-28
#### 🚀 Features
- ✨ implement note markdown for task - (a310a63) - Zen Kiet

- - -

## v1.8.0 - 2026-08-28
#### 🚀 Features
- ✨ improve  notification for task - (fed4eb2) - Zen Kiet
- ✨ adopt structured logging with log/slog - (387b661) - Zen Kiet
#### 📝 Documentation
- 📝 add AGENTS.md for autonomous agents - (b59b402) - Zen Kiet, *factory-droid[bot]*
#### 🔧 Chores & Maintenance
- 🔖 add structured GitHub issue forms - (710c842) - Zen Kiet, *factory-droid[bot]*
- 🔖 add pull request template - (c9a7c39) - Zen Kiet, *factory-droid[bot]*

- - -

## v1.7.0 - 2026-08-25
#### 🚀 Features
- ✨ implement dev status - (3d5e164) - Zen Kiet

- - -

## v1.6.1 - 2026-08-24
#### ♻️ Code Refactoring
- ♻️ improve notification response - (86e8363) - Zen Kiet

- - -

## v1.6.0 - 2026-08-24
#### 🚀 Features
- ✨ implement fcm - (e569b05) - *zenkiet*

- - -

## v1.5.0 - 2026-08-23
#### 🚀 Features
- ✨ implement metrics monitor - (5d02ba4) - Zen Kiet

- - -

## v1.4.0 - 2026-08-22
#### 🚀 Features
- ✨ implement RBAC feature - (cd7025c) - Zen Kiet

- - -

## v1.3.0 - 2026-08-22
#### 🚀 Features
- ✨ implement notification gateway router - (2d57495) - Zen Kiet

- - -

## v1.2.0 - 2026-08-19
#### 🚀 Features
- ✨ add project task form defaults - (09cb533) - Zen Kiet

- - -

## v1.1.1 - 2026-08-19
#### 👷 Continuous Integration
- 👷 setup publish workflow pipeline - (02cd3e5) - *zenkiet*

- - -

## v1.1.0 - 2026-08-19
#### 🚀 Features
- ✨ add CI deploy API tokens - (30bc132) - Zen Kiet, *factory-droid[bot]*
#### 👷 Continuous Integration
- 👷 implement bump version tool - (6cb81f4) - *zenkiet*
- 👷 improve Dockerfile - (5d73af1) - *zenkiet*

- - -

Changelog generated by [cocogitto](https://github.com/cocogitto/cocogitto).