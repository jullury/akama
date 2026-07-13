## [3.9.3](https://github.com/jullury/akama/compare/v3.9.2...v3.9.3) (2026-07-13)


### Bug Fixes

* **job:** reuse plan-phase workspace instead of re-cloning ([9f436ab](https://github.com/jullury/akama/commit/9f436abbe7561517d7824bd8d191200e3413f854))

## [3.9.2](https://github.com/jullury/akama/compare/v3.9.1...v3.9.2) (2026-07-13)


### Bug Fixes

* pull daemon image from GHCR before falling back to local build ([d046189](https://github.com/jullury/akama/commit/d046189e75280eb596f69972b85b43c34eaabc85))

## [3.9.1](https://github.com/jullury/akama/compare/v3.9.0...v3.9.1) (2026-07-13)


### Bug Fixes

* add Postgres data volume for container management ([118cd79](https://github.com/jullury/akama/commit/118cd7988e1e073e4ea3d5088410ab2399951a9e))

# [3.9.0](https://github.com/jullury/akama/compare/v3.8.5...v3.9.0) (2026-07-13)


### Bug Fixes

* ensure pnpm is available in container ([8a81a1f](https://github.com/jullury/akama/commit/8a81a1f4a46293ababf90261fbf1a777655ca3e7))


### Features

* run clarifying question generation asynchronously ([4d05b4a](https://github.com/jullury/akama/commit/4d05b4a78a2989eb1bcd65696333b66b7b794d3d))

## [3.8.5](https://github.com/jullury/akama/compare/v3.8.4...v3.8.5) (2026-07-12)


### Bug Fixes

* log bot polling loop only after first successful GetUpdates ([13bdc02](https://github.com/jullury/akama/commit/13bdc02f497f0bbc1ffd96135ee2e8465fda1cf0))

## [3.8.4](https://github.com/jullury/akama/compare/v3.8.3...v3.8.4) (2026-07-12)


### Bug Fixes

* **start:** skip daemon image build when image is up to date ([443f7ef](https://github.com/jullury/akama/commit/443f7ef579472a6d9cefbc20173f8166f3b4398d))

## [3.8.3](https://github.com/jullury/akama/compare/v3.8.2...v3.8.3) (2026-07-12)


### Bug Fixes

* **docker:** only request NVIDIA GPU when nvidia-smi is available on host ([fea93e9](https://github.com/jullury/akama/commit/fea93e9ce13f86d1a22c8db9317670e66da4ab9a))

## [3.8.2](https://github.com/jullury/akama/compare/v3.8.1...v3.8.2) (2026-07-12)


### Bug Fixes

* **docker:** use Node.js 22 from NodeSource for Claude Code compatibility ([db0fba4](https://github.com/jullury/akama/commit/db0fba4954c7aec579699ecfdafe2f90eee21a7b))

## [3.8.1](https://github.com/jullury/akama/compare/v3.8.0...v3.8.1) (2026-07-12)


### Bug Fixes

* **docker:** fix RTK binary install in container image ([ee6cd72](https://github.com/jullury/akama/commit/ee6cd7255deb9a7446533bd186fe0fa32acd73bc))

# [3.8.0](https://github.com/jullury/akama/compare/v3.7.4...v3.8.0) (2026-07-12)


### Features

* add RTK token optimizer to Docker image ([97a9de5](https://github.com/jullury/akama/commit/97a9de5f8479da057aa82932a55a1c036e9efc3e))
* **knowledge:** enhance knowledge base for better issue resolution ([7ac934a](https://github.com/jullury/akama/commit/7ac934a4275307d4dd967b4a7e6da5af787046d9))

## [3.7.4](https://github.com/jullury/akama/compare/v3.7.3...v3.7.4) (2026-07-10)


### Bug Fixes

* sync embedded skills with host ~/.opencode/skills/ ([2a12556](https://github.com/jullury/akama/commit/2a12556811d45a08bb86648874757c507257c4ce))

## [3.7.3](https://github.com/jullury/akama/compare/v3.7.2...v3.7.3) (2026-05-21)


### Bug Fixes

* improve follow-up state validation and cleanup ([fd8e7a6](https://github.com/jullury/akama/commit/fd8e7a68fe3b89afcba81891078c9d51b77f435f))

## [3.7.2](https://github.com/jullury/akama/compare/v3.7.1...v3.7.2) (2026-05-21)


### Bug Fixes

* fix(job): resolve mise PATH lookup and Ollama context overflow ([9a1b08c](https://github.com/jullury/akama/commit/9a1b08c8f09ed193da445252c59dd31f483e391e))

## [3.7.1](https://github.com/jullury/akama/compare/v3.7.0...v3.7.1) (2026-05-21)


### Bug Fixes

* use stdin to avoid command ARG_MAX error ([674db4e](https://github.com/jullury/akama/commit/674db4e457c252b1ce2a3c5d9d5127828dda3ee2))

# [3.7.0](https://github.com/jullury/akama/compare/v3.6.2...v3.7.0) (2026-05-20)


### Features

* enable NVIDIA GPU for Ollama container ([758deca](https://github.com/jullury/akama/commit/758deca98d37f213f73c1f8035f52d199993155d))
* stage host binary update from inside Docker ([244044a](https://github.com/jullury/akama/commit/244044a766852b2545e6c690f97a9c1f57ac7010))

## [3.6.2](https://github.com/jullury/akama/compare/v3.6.1...v3.6.2) (2026-05-20)


### Bug Fixes

* surface model pull errors during startup ([c42afe0](https://github.com/jullury/akama/commit/c42afe0ddd073a8db4e42802a286a161e307261f))

## [3.6.1](https://github.com/jullury/akama/compare/v3.6.0...v3.6.1) (2026-05-20)


### Bug Fixes

* use detached exec for model pull ([d4ea53a](https://github.com/jullury/akama/commit/d4ea53a02d95e3784e885762d2da721242340a54))

# [3.6.0](https://github.com/jullury/akama/compare/v3.5.0...v3.6.0) (2026-05-20)


### Features

* embed skills in binary and support opencode agent ([9494a2f](https://github.com/jullury/akama/commit/9494a2fe018e0b809c151f9447bf0ca43b52ac0d))

# [3.5.0](https://github.com/jullury/akama/compare/v3.4.0...v3.5.0) (2026-05-20)


### Features

* add help, metrics, preview, quickfix, and retry_all bot commands ([7e037a8](https://github.com/jullury/akama/commit/7e037a8faffafd1e347df8c6bca107a83f27693e))

# [3.4.0](https://github.com/jullury/akama/compare/v3.3.1...v3.4.0) (2026-05-20)


### Features

* add interactive branch selection via inline keyboards in Telegr... ([d67058b](https://github.com/jullury/akama/commit/d67058bf821df9d4aeb5a277ca526ff318c394e9))

## [3.3.1](https://github.com/jullury/akama/compare/v3.3.0...v3.3.1) (2026-05-20)


### Bug Fixes

* chmod plan workspaces to 0755 so agent can chdir as uid 1000 ([ddbddef](https://github.com/jullury/akama/commit/ddbddef2535ac8b398ff81902f9c9feca9e475f9))

# [3.3.0](https://github.com/jullury/akama/compare/v3.2.0...v3.3.0) (2026-05-20)


### Features

* skip clarifying questions when agent output is insufficient ([a4d4e6f](https://github.com/jullury/akama/commit/a4d4e6ff5b43f449777b7322ef66d4d332c41a0a))

# [3.2.0](https://github.com/jullury/akama/compare/v3.1.3...v3.2.0) (2026-05-20)


### Bug Fixes

* remove duplicate job creation from proceedWithPlan ([dbd9c92](https://github.com/jullury/akama/commit/dbd9c92b015c21b8ac857673b142e4677982a64e))


### Features

* add multi-repo context to clarifying questions and plan prompts ([ddc07a0](https://github.com/jullury/akama/commit/ddc07a02172c2c3a814a2f5f6235c1f2e10c2537))

## [3.1.3](https://github.com/jullury/akama/compare/v3.1.2...v3.1.3) (2026-05-19)


### Bug Fixes

* case-insensitive clarifying question parsing, add Docker build cache ([fb38dae](https://github.com/jullury/akama/commit/fb38dae2b03b9512c872abde98e6033682077ebd))

## [3.1.2](https://github.com/jullury/akama/compare/v3.1.1...v3.1.2) (2026-05-19)


### Bug Fixes

* pass CLI version to daemon cross-compile so daemon reports correct version ([2a0147b](https://github.com/jullury/akama/commit/2a0147be091a319d5cf94da784b06be1825d9bb1))

## [3.1.1](https://github.com/jullury/akama/compare/v3.1.0...v3.1.1) (2026-05-19)


### Bug Fixes

* restore golang:1.26-bookworm builder to match go.mod 1.26.1 ([5072a0f](https://github.com/jullury/akama/commit/5072a0f126c19780a334b95c533ef7c2cd0f79d6))

# [3.1.0](https://github.com/jullury/akama/compare/v3.0.4...v3.1.0) (2026-05-19)


### Features

* merge worker into daemon, privilege-drop agents, fix container lifecycle ([819f919](https://github.com/jullury/akama/commit/819f91916d778b612a4b579a18f693f14355929f))

## [3.0.4](https://github.com/jullury/akama/compare/v3.0.3...v3.0.4) (2026-05-19)


### Bug Fixes

* increase Docker client timeout to 5m for image pulls ([845b785](https://github.com/jullury/akama/commit/845b78519018dc12f96e8fcecd69889e763ecbbe))

## [3.0.3](https://github.com/jullury/akama/compare/v3.0.2...v3.0.3) (2026-05-19)


### Bug Fixes

* bump builder image to golang:1.26-bookworm to match go.mod ([4e17c95](https://github.com/jullury/akama/commit/4e17c958a5710ded704c23c49de9f60bbe659624))

## [3.0.2](https://github.com/jullury/akama/compare/v3.0.1...v3.0.2) (2026-05-19)


### Bug Fixes

* use Docker build secrets instead of ARG for OAuth credentials ([08e1dc0](https://github.com/jullury/akama/commit/08e1dc0398ee7035f2152449ba7fb6da53a1898a))

## [3.0.1](https://github.com/jullury/akama/compare/v3.0.0...v3.0.1) (2026-05-19)


### Bug Fixes

* trigger patch release ([6bf5901](https://github.com/jullury/akama/commit/6bf590113fea61d3e97d0bba832444dfe2287e08))

# [3.0.0](https://github.com/jullury/akama/compare/v2.19.0...v3.0.0) (2026-05-19)


* feat!: convert to Docker-based architecture with PostgreSQL and semantic knowledge ([73b784c](https://github.com/jullury/akama/commit/73b784c56c06d5e6db239410d79e3e1e7c0598f9))


### Features

* add Dockerfile and Docker manager for containerized infrastructure ([bbcbfbf](https://github.com/jullury/akama/commit/bbcbfbfe8bba3e00001ff5b16655eef7eda2246d))
* add semantic knowledge base for issue similarity retrieval ([627b9e5](https://github.com/jullury/akama/commit/627b9e53a33a550bce9daa316b9fa7cc9d0b9e75))
* integrate knowledge base into job runner and agent prompt ([0e7b795](https://github.com/jullury/akama/commit/0e7b795490ae891a37f8582a6d3628ae86a5ee49))
* replace SQLite with PostgreSQL, add pgvector support ([68c64dc](https://github.com/jullury/akama/commit/68c64dca68f06c5ef3629af8207bbbdcd5b852c3))


### BREAKING CHANGES

* Akama now runs entirely in Docker containers. Native
process management (ForkDaemon, PID files, SIGTERM) is replaced with
Docker lifecycle commands. The daemon/ package is removed.

- Replace SQLite with PostgreSQL (pgvector) running in Docker
- Add akama-postgres (pgvector/pgvector:pg16), akama-ollama (ollama/ollama),
  akama-daemon (locally built) containers on akama-net bridge network
- Add semantic knowledge base: Ollama nomic-embed-text embeddings stored
  in job_embeddings table with pgvector cosine similarity search
- Add knowledge retrieval before each agent run (top-3 similar past jobs)
- Rewrite all CLI commands for Docker lifecycle:
  akama init  — Docker build + pull + provision phase
  akama start — ensures infra containers + starts daemon
  akama stop  — graceful container stop with wait loop
  akama logs  — docker logs wrapper (--follow/--tail)
  akama restart — Docker lifecycle restart
  akama update — rebuild image + restart daemon
  akama db {start,stop,reset,status} — PostgreSQL management
  akama migrate — one-shot SQLite to PostgreSQL migration
- Config: db_path removed, postgres_url and ollama_url added
- Docker SDK 28.5.2+incompatible for container orchestration
- modernc.org/sqlite retained for migration tool only

# [2.19.0](https://github.com/jullury/akama/compare/v2.18.0...v2.19.0) (2026-05-19)


### Features

* add 7 missing UX features ([4b65846](https://github.com/jullury/akama/commit/4b65846151b9490f0fa496c0895ea2b275444ba1))

# [2.18.0](https://github.com/jullury/akama/compare/v2.17.0...v2.18.0) (2026-05-19)


### Features

* security, reliability, observability, and test coverage improvements ([0155cd3](https://github.com/jullury/akama/commit/0155cd3913d348e2c5e6715e63a3fed88bc11e4b))

# [2.17.0](https://github.com/jullury/akama/compare/v2.16.3...v2.17.0) (2026-05-18)


### Features

* add version command, restart command, and build metadata injection ([d63a063](https://github.com/jullury/akama/commit/d63a0636ab2fea5fb7e1968def3d2bad07265e10))

## [2.16.3](https://github.com/jullury/akama/compare/v2.16.2...v2.16.3) (2026-05-18)


### Bug Fixes

* **provider:** add workflow scope to GitHub OAuth and surface workflow push errors ([81b302a](https://github.com/jullury/akama/commit/81b302ad3936cc3048e3b585980355d0319931ee))

## [2.16.2](https://github.com/jullury/akama/compare/v2.16.1...v2.16.2) (2026-05-18)


### Bug Fixes

* **bot:** send plan in chunks to avoid Telegram 4096-char limit ([f3335d4](https://github.com/jullury/akama/commit/f3335d487492b5b3fed2feafbcc34c9ebdbb05ec))

## [2.16.1](https://github.com/jullury/akama/compare/v2.16.0...v2.16.1) (2026-05-18)


### Bug Fixes

* **bot:** refresh token from connections before posting plan comment ([5a20113](https://github.com/jullury/akama/commit/5a2011365b16b4c7e6a00435410d0d202d539138))

# [2.16.0](https://github.com/jullury/akama/compare/v2.15.0...v2.16.0) (2026-05-18)


### Bug Fixes

* **bot:** fix plan review hang — async regen with heartbeat and guard state ([80d3cba](https://github.com/jullury/akama/commit/80d3cbaa955a30784ad5ed29cf7448c6666e0712))
* **bot:** respond gracefully when plan:confirm tapped during await_plan_regen ([36384e1](https://github.com/jullury/akama/commit/36384e179f6e306eb18b86c1375c6a00d578901f))


### Features

* **bot:** clone repo before plan generation for codebase context ([619b6f5](https://github.com/jullury/akama/commit/619b6f59212a0f7cdc3ff3afb8cb6ef2f908ffbc))

# [2.15.0](https://github.com/jullury/akama/compare/v2.14.0...v2.15.0) (2026-05-18)


### Bug Fixes

* apply changes ([dec16b9](https://github.com/jullury/akama/commit/dec16b909487ea42584766e795b9705bc5ff3fd5))


### Features

* add clarifying questions and plan review workflow for issue triage ([5402674](https://github.com/jullury/akama/commit/54026741e14c7da3ad3bc92de95fc123f96bd863))

# [2.14.0](https://github.com/jullury/akama/compare/v2.13.1...v2.14.0) (2026-05-18)


### Bug Fixes

* fall back to "main" when default branch is empty ([fabe4ad](https://github.com/jullury/akama/commit/fabe4ad9e6de04e13c8ee6b18db6796355b99495))


### Features

* add per-repo branch selection before issue description ([10c44b7](https://github.com/jullury/akama/commit/10c44b71bca71c235fe39e17978e3dc76fbc8ec4))

## [2.13.1](https://github.com/jullury/akama/compare/v2.13.0...v2.13.1) (2026-05-18)


### Bug Fixes

* use url.Parse for repository URL parsing in workspace path const... ([10e1951](https://github.com/jullury/akama/commit/10e1951e26c99d9fede7597f0511ee47c179ccc7))

# [2.13.0](https://github.com/jullury/akama/compare/v2.12.0...v2.13.0) (2026-05-18)


### Features

* add mise-based toolchain management with make setup ([19b147c](https://github.com/jullury/akama/commit/19b147ccc53dfd7ef89e30470b81617fdb965200))
* automatically install mise tools after clone and before job exe... ([85ae4f3](https://github.com/jullury/akama/commit/85ae4f357da6f2460c0c9dd8d81c62f9460fb0e1))

# [2.12.0](https://github.com/jullury/akama/compare/v2.11.8...v2.12.0) (2026-05-18)


### Features

* support multi-repo selection for new issues with group-aware cl... ([f838da4](https://github.com/jullury/akama/commit/f838da4301ca219b138e36adf1449e2ef04cd261))

## [2.11.8](https://github.com/jullury/akama/compare/v2.11.7...v2.11.8) (2026-05-08)


### Bug Fixes

* **bot:** reset Telegram session state on startup ([dc77dc2](https://github.com/jullury/akama/commit/dc77dc2036924ff859c98e07d35b79ec11b2617c))

## [2.11.7](https://github.com/jullury/akama/compare/v2.11.6...v2.11.7) (2026-05-08)


### Bug Fixes

* **update:** download before stopping daemon ([c2061eb](https://github.com/jullury/akama/commit/c2061eb89b47dc555f1ec6151f16eb066d979b76))

## [2.11.6](https://github.com/jullury/akama/compare/v2.11.5...v2.11.6) (2026-05-08)


### Bug Fixes

* **bot:** auto-recover from polling conflict ([f04e43e](https://github.com/jullury/akama/commit/f04e43e739502000343eeac2bb55957ce72c88f6))

## [2.11.5](https://github.com/jullury/akama/compare/v2.11.4...v2.11.5) (2026-05-08)


### Bug Fixes

* **bot:** prevent Telegram polling conflict on daemon restart ([6127ed5](https://github.com/jullury/akama/commit/6127ed55eb3f9eef91e58f361f42a477afdfffed))

## [2.11.4](https://github.com/jullury/akama/compare/v2.11.3...v2.11.4) (2026-05-08)


### Bug Fixes

* **docker:** flush stale getUpdates long-poll on bot startup ([3b03330](https://github.com/jullury/akama/commit/3b03330f8ca66c7b2cf94825bf517b1f7fd969d1))

## [2.11.3](https://github.com/jullury/akama/compare/v2.11.2...v2.11.3) (2026-05-08)


### Bug Fixes

* **docker:** use SIGTERM instead of syscall.Exec for update restart ([a27ba5d](https://github.com/jullury/akama/commit/a27ba5d63e07f3d4d8eb6a41ecf9c58f872466de))

## [2.11.2](https://github.com/jullury/akama/compare/v2.11.1...v2.11.2) (2026-05-08)


### Bug Fixes

* **docker:** restart daemon in-place and preserve updated binary ([96af4ed](https://github.com/jullury/akama/commit/96af4ed5a83e161dabe3c2f7e86310e00e29cb0c))

## [2.11.1](https://github.com/jullury/akama/compare/v2.11.0...v2.11.1) (2026-05-08)


### Bug Fixes

* **agent:** include stdout in error messages when agent fails ([560e2b2](https://github.com/jullury/akama/commit/560e2b2848502913ead70f13dbd4046b27ce457f))

# [2.11.0](https://github.com/jullury/akama/compare/v2.10.0...v2.11.0) (2026-05-08)


### Features

* add --version flag and auto-upgrade volume binary from image seed ([06b5b5a](https://github.com/jullury/akama/commit/06b5b5a2dd275796302a016c82a953d93b57a36b))

# [2.10.0](https://github.com/jullury/akama/compare/v2.9.0...v2.10.0) (2026-05-08)


### Features

* **docker:** run container as non-root user with agent seeding ([9d42a89](https://github.com/jullury/akama/commit/9d42a89a2c6266f5b8b0185663fa0eb81ea260b6))

# [2.9.0](https://github.com/jullury/akama/compare/v2.8.1...v2.9.0) (2026-05-07)


### Features

* upload images to provider hosting instead of embedding as base64 ([9fe1ebb](https://github.com/jullury/akama/commit/9fe1ebb401abc27ac545d083f3c4138218273a38))

## [2.8.1](https://github.com/jullury/akama/compare/v2.8.0...v2.8.1) (2026-05-07)


### Bug Fixes

* prevent users from deleting themselves instead of blocking admin... ([6e9dc53](https://github.com/jullury/akama/commit/6e9dc534aaf67ee3247735766b6e2a226283b6c9))

# [2.8.0](https://github.com/jullury/akama/compare/v2.7.0...v2.8.0) (2026-05-07)


### Features

* embed images as base64 data URIs in issue body ([5a3c353](https://github.com/jullury/akama/commit/5a3c3530473bd2cc886ad70ec0da08fb1a9834fb))
* enrich issue body with embedded images and comments ([25419b8](https://github.com/jullury/akama/commit/25419b81d2110d774bd92c05dc1a9214c02f12af))

# [2.7.0](https://github.com/jullury/akama/compare/v2.6.1...v2.7.0) (2026-05-07)


### Features

* run Docker container as non-root user for security ([f34a253](https://github.com/jullury/akama/commit/f34a253791969bb61e0108a88bc59bf479910a90))

## [2.6.1](https://github.com/jullury/akama/compare/v2.6.0...v2.6.1) (2026-05-07)


### Bug Fixes

* bump version to 2.6.0 ([c25ea90](https://github.com/jullury/akama/commit/c25ea90066aec351d4970fdb02088652eb8819bd))

# [2.6.0](https://github.com/jullury/akama/compare/v2.5.0...v2.6.0) (2026-05-07)


### Features

* **bot:** add storage package dependency to bot ([9702e80](https://github.com/jullury/akama/commit/9702e8037d783f3f58708d18857fa9d528106256))

# [2.5.0](https://github.com/jullury/akama/compare/v2.4.1...v2.5.0) (2026-05-07)


### Features

* add Telegram bot user authorization and admin user management ([a7d86d7](https://github.com/jullury/akama/commit/a7d86d743d533ebe0a5c4c856b137be8e34200a8))
* restrict admin commands and user management to admin users only ([fb70698](https://github.com/jullury/akama/commit/fb7069870c0805405b27cd982ae5f8f85eaf8467))

## [2.4.1](https://github.com/jullury/akama/compare/v2.4.0...v2.4.1) (2026-05-06)


### Bug Fixes

* **agent:** fix opencode npm update and install commands ([bf1a2cf](https://github.com/jullury/akama/commit/bf1a2cfc66583ebda02a4d46419b0ac9217e8ee9))

# [2.4.0](https://github.com/jullury/akama/compare/v2.3.2...v2.4.0) (2026-05-06)


### Features

* **docker:** add Docker deployment with compose setup ([f7ead2a](https://github.com/jullury/akama/commit/f7ead2ad354f1762a175a20bc3b2ec46eaa20610))

## [2.3.2](https://github.com/jullury/akama/compare/v2.3.1...v2.3.2) (2026-05-06)


### Bug Fixes

* apply changes ([07dc58f](https://github.com/jullury/akama/commit/07dc58f2bd14fe1f3bf665ede83afe2014870025))

## [2.3.1](https://github.com/jullury/akama/compare/v2.3.0...v2.3.1) (2026-05-06)


### Bug Fixes

* **bot:** resolve /done command routing conflict ([591f6a9](https://github.com/jullury/akama/commit/591f6a98fd5c052493b0b436d2aed58598f7b4ed))

# [2.3.0](https://github.com/jullury/akama/compare/v2.2.1...v2.3.0) (2026-05-06)


### Features

* **skills:** add raw skill support and enhance skill management ([4f0c184](https://github.com/jullury/akama/commit/4f0c1841f390c14354212baf0a006a62d4517e3c))

## [2.2.1](https://github.com/jullury/akama/compare/v2.2.0...v2.2.1) (2026-05-06)


### Bug Fixes

* **skills:** add senior backend, frontend, and devops skills ([1382705](https://github.com/jullury/akama/commit/1382705ff1232a62dc4870ac76153afa323398a3))

# [2.2.0](https://github.com/jullury/akama/compare/v2.1.0...v2.2.0) (2026-05-06)


### Features

* **bot:** add image attachment support for Telegram issue creation ([cb9d0b1](https://github.com/jullury/akama/commit/cb9d0b18963907ceb5cc9bb53ee757e18bd40f3e))

# [2.1.0](https://github.com/jullury/akama/compare/v2.0.0...v2.1.0) (2026-05-06)


### Features

* **skills:** add required skills and automatic prompt injection ([c03d744](https://github.com/jullury/akama/commit/c03d744cddc47fb6d4cd24889c85aa043e5d2776))

# [2.0.0](https://github.com/jullury/akama/compare/v1.17.1...v2.0.0) (2026-05-06)


* feat!: add skillhub.club integration for extensible agent capabilities ([e5a18ee](https://github.com/jullury/akama/commit/e5a18eec306c9b4a38bec11d269fbe5d9d60ad0d))


### BREAKING CHANGES

* Introduce new /skills command and skill installation system

Add comprehensive skill management system that allows users to browse and install
skills from skillhub.club, significantly expanding agent capabilities.

Key features:
- Built-in skill catalog with 12 pre-curated skills
- Interactive skill browser with inline keyboard navigation
- Custom skill installation by ID
- Automatic skill installation across all agents (claude, opencode)
- New conversation state handling for skill ID input

New commands:
- /skills — browse and install skillhub.club skills
- + Custom skill by ID — install skills by identifier

Files modified:
- cmd/init.go: Update initialization for skill system
- internal/agent/skills.go: New skill management package
- internal/bot/commands.go: Add skills command and installation logic
- internal/bot/router.go: Add routing for skills commands and callbacks

## [1.17.1](https://github.com/jullury/akama/compare/v1.17.0...v1.17.1) (2026-05-06)


### Bug Fixes

* **update:** simplify daemon restart process ([d649f35](https://github.com/jullury/akama/commit/d649f350214bbb9df64e19c62abe24e90b33339c))

# [1.17.0](https://github.com/jullury/akama/compare/v1.16.0...v1.17.0) (2026-05-06)


### Features

* **bot:** add /update command and standardize command descriptions ([8e464c9](https://github.com/jullury/akama/commit/8e464c96a9b153670ec6ec4ae40debe740fa0e58))

# [1.16.0](https://github.com/jullury/akama/compare/v1.15.0...v1.16.0) (2026-05-06)


### Features

* save pending actions and retry after token refresh ([551e9f7](https://github.com/jullury/akama/commit/551e9f7234ebb2b9fe08de93783b8706075abe51))

# [1.15.0](https://github.com/jullury/akama/compare/v1.14.1...v1.15.0) (2026-05-06)


### Features

* **bot:** add /update command to update server binary ([098812a](https://github.com/jullury/akama/commit/098812aa265aecdbcefa08db5eb75411897e2bf3))

## [1.14.1](https://github.com/jullury/akama/compare/v1.14.0...v1.14.1) (2026-05-06)


### Bug Fixes

* **bot:** standardize command naming and fix callback routing ([dd05c6c](https://github.com/jullury/akama/commit/dd05c6c1738b0769cc1206ac2c1867c5793120b4))

# [1.14.0](https://github.com/jullury/akama/compare/v1.13.0...v1.14.0) (2026-05-06)


### Features

* add agent update command and refactor install logic to agent pkg ([490e223](https://github.com/jullury/akama/commit/490e223d5330c21a294f817864f2e059430ad2e4))

# [1.13.0](https://github.com/jullury/akama/compare/v1.12.0...v1.13.0) (2026-05-06)


### Features

* **bot:** replace issues filter text input with inline buttons ([4598345](https://github.com/jullury/akama/commit/45983459208ecbca0def7c9fa379376fcdf62b68))

# [1.12.0](https://github.com/jullury/akama/compare/v1.11.2...v1.12.0) (2026-05-05)


### Features

* **bot:** register Telegram commands on startup ([218b509](https://github.com/jullury/akama/commit/218b50925022809ca938cb740890850c1d5d045f))

## [1.11.2](https://github.com/jullury/akama/compare/v1.11.1...v1.11.2) (2026-05-05)


### Bug Fixes

* **bot:** reorder command handlers to fix prefix matching conflicts ([c8c959f](https://github.com/jullury/akama/commit/c8c959f07d5768ef01887c3e43c87643c4c7acf1))

## [1.11.1](https://github.com/jullury/akama/compare/v1.11.0...v1.11.1) (2026-05-05)


### Bug Fixes

* **git:** update askpass gitignore and improve appendIfMissing logic ([42b200d](https://github.com/jullury/akama/commit/42b200d82a4c2eaf2c9fa7ebe5367987e3718a73))

# [1.11.0](https://github.com/jullury/akama/compare/v1.10.0...v1.11.0) (2026-05-05)


### Features

* **bot:** add /connection delete to remove individual connections ([64eca6b](https://github.com/jullury/akama/commit/64eca6bb12347507b6f9bfe385ecc9b6c37fbe44))

# [1.10.0](https://github.com/jullury/akama/compare/v1.9.0...v1.10.0) (2026-05-05)


### Features

* allow follow-up for jobs with 'updating' status ([6fa5144](https://github.com/jullury/akama/commit/6fa51449a46764b40dde96fb271d0d233002ee49))
* **bot:** add /followup command for pr_created jobs ([0a04907](https://github.com/jullury/akama/commit/0a04907971536268f7c23fa6a64091e393b784ce))

# [1.9.0](https://github.com/jullury/akama/compare/v1.8.2...v1.9.0) (2026-05-05)


### Features

* paginate /status command with inline navigation ([750e32a](https://github.com/jullury/akama/commit/750e32a2ce39859555767787d6361512e0003774))

## [1.8.2](https://github.com/jullury/akama/compare/v1.8.1...v1.8.2) (2026-05-05)


### Bug Fixes

* use repo name instead of provider in agent progress notification ([1817660](https://github.com/jullury/akama/commit/1817660564fe58ce73d822343c98aa17a8192b3e))

## [1.8.1](https://github.com/jullury/akama/compare/v1.8.0...v1.8.1) (2026-05-05)


### Bug Fixes

* handle authentication errors and refresh connection tokens ([e77ab3c](https://github.com/jullury/akama/commit/e77ab3c31b8048ef04d7a6c9d4debb5260d5e2ac))

# [1.8.0](https://github.com/jullury/akama/compare/v1.7.0...v1.8.0) (2026-05-05)


### Features

* **bot:** show owner/repo in status output ([1fc3c4d](https://github.com/jullury/akama/commit/1fc3c4d285afd25cc98755d901f0bffb850a75c8))

# [1.7.0](https://github.com/jullury/akama/compare/v1.6.2...v1.7.0) (2026-05-05)


### Features

* implement smart chunking for long agent outputs ([5098f7d](https://github.com/jullury/akama/commit/5098f7d5f6561292be49e405e0c8fecc40b30a88))

## [1.6.2](https://github.com/jullury/akama/compare/v1.6.1...v1.6.2) (2026-05-05)


### Bug Fixes

* improve opencode output parsing for better terminal display ([ee8c5e6](https://github.com/jullury/akama/commit/ee8c5e6ca87492a61765e4971d97122828377513))

## [1.6.1](https://github.com/jullury/akama/compare/v1.6.0...v1.6.1) (2026-05-05)


### Bug Fixes

* **install:** unify package manager fallback logic and improve error handling ([df77283](https://github.com/jullury/akama/commit/df772832aa778c2e97ab1c53b40cd500baca80f6))

# [1.6.0](https://github.com/jullury/akama/compare/v1.5.1...v1.6.0) (2026-05-05)


### Features

* add GitLab work_items support and persist default branch ([87d4c9d](https://github.com/jullury/akama/commit/87d4c9d22ced55596f830e5270482e835af8a217))

## [1.5.1](https://github.com/jullury/akama/compare/v1.5.0...v1.5.1) (2026-05-05)


### Bug Fixes

* **git:** use explicit refspec for fetch before force-with-lease ([644aad0](https://github.com/jullury/akama/commit/644aad096003563ef82e9f0dae52e6ea7dc3b4be))

# [1.5.0](https://github.com/jullury/akama/compare/v1.4.0...v1.5.0) (2026-05-05)


### Features

* prompt branch confirmation on first issue per repo ([5c5612e](https://github.com/jullury/akama/commit/5c5612effd8ff7f2f3c480dfe6de3d0e7925abe9))
* prompt branch confirmation on first issue per repo ([8e5f4ce](https://github.com/jullury/akama/commit/8e5f4ce7f7a61a69192779759f9efe97f0b7bf2c))
* prompt branch confirmation on first issue per repo ([2c124bf](https://github.com/jullury/akama/commit/2c124bf21ff9ad1338864016371d4b545653bfbe))
* store and use repo default branch for PR/MR creation ([0d8e557](https://github.com/jullury/akama/commit/0d8e5576e1de8f40b39b654f48d45eadd2d9368a))

# [1.4.0](https://github.com/jullury/akama/compare/v1.3.0...v1.4.0) (2026-05-05)


### Features

* send agent output to user after each agent run ([5a4469f](https://github.com/jullury/akama/commit/5a4469f858d0433d4e8b5a09da5a661c28c29976))

# [1.3.0](https://github.com/jullury/akama/compare/v1.2.1...v1.3.0) (2026-05-05)


### Features

* **bot:** add /help, /queue, /logs, /retry, /cancel commands ([407941e](https://github.com/jullury/akama/commit/407941ee9b173352e5112a6c9819e10a12763d75))

## [1.2.1](https://github.com/jullury/akama/compare/v1.2.0...v1.2.1) (2026-05-05)


### Bug Fixes

* **git:** refresh tracking ref before force-with-lease push ([652f0d9](https://github.com/jullury/akama/commit/652f0d9bf3c3159dab510a48452cdfc67b91270c))

# [1.2.0](https://github.com/jullury/akama/compare/v1.1.1...v1.2.0) (2026-05-05)


### Features

* add agent selection to user configuration ([ff233ad](https://github.com/jullury/akama/commit/ff233adc500bee8b26420400f7a90ed7b5987159))
* auto-install claude and opencode agents during init ([57064fd](https://github.com/jullury/akama/commit/57064fd7cdac9e013914920623dfe9319c02e4af))

## [1.1.1](https://github.com/jullury/akama/compare/v1.1.0...v1.1.1) (2026-05-05)


### Bug Fixes

* apply changes ([ddfe0d3](https://github.com/jullury/akama/commit/ddfe0d30cd9cf7e870c07b4982e26a272ff7fe5c))

# [1.1.0](https://github.com/jullury/akama/compare/v1.0.0...v1.1.0) (2026-05-05)


### Features

* add log rotation and enhance logs command with follow/all flags ([4ac22b9](https://github.com/jullury/akama/commit/4ac22b9084e16d1c11e85ffb9ee28b4948ce2e78))

# 1.0.0 (2026-05-05)


### Bug Fixes

* **git:** resolve clone failure when destination exists by removing it first ([c0b82d5](https://github.com/jullury/akama/commit/c0b82d559e7f56340b6e5a7e7a16494f35547506))
* manage conversation state for job completion and input needs ([d69ddc2](https://github.com/jullury/akama/commit/d69ddc278926a58943ed5d98f2aa08f46de0dfdb))


### Features

* activate all workflows ([4df029b](https://github.com/jullury/akama/commit/4df029b562b4cea29f67f869cc5fc47b66d8ccd5))
* add /newissue command to create and process issues ([c320bc8](https://github.com/jullury/akama/commit/c320bc8b58b3623bde1ea9c6304d18e862648b3d))
* add debug logging and fix issue URL parsing ([f6e2d48](https://github.com/jullury/akama/commit/f6e2d48e39212fb9a70b3d73c76eaaeebc6b2064))
* add dynamic model fetching and pagination for AI providers ([d0624c0](https://github.com/jullury/akama/commit/d0624c09dcaaac26c78c156b27f08f0a09af2706))
* add initial project structure with Docker setup and n8n workflows ([b6e8d73](https://github.com/jullury/akama/commit/b6e8d730425811f4d11d06f446e53461a8b2a084))
* add interactive agent workflow with user input handling ([ee80fdf](https://github.com/jullury/akama/commit/ee80fdf3860a5606b6704853412a0dab7283670a))
* add N8N_API_KEY for automated workflow activation ([fae04d2](https://github.com/jullury/akama/commit/fae04d25910b40be5b8d1f4588fe5255231e0106))
* add opencode error detection and retry agent runs ([fa07571](https://github.com/jullury/akama/commit/fa075718d63a726547b735d61fc5170e98eae016))
* add Telegram bot token configuration and comprehensive project documentation ([524665d](https://github.com/jullury/akama/commit/524665d17aa9acbce59ff1e71a21e0c65f6e4a75))
* add user git config support and recover interrupted jobs ([502a3c5](https://github.com/jullury/akama/commit/502a3c53330e2989dca9d84b9915bb80c2eaa460))
* add version support and adopt semantic-release workflow ([e4c28b7](https://github.com/jullury/akama/commit/e4c28b7f58af17aea78ee4d77214422df77a85b6))
* **bot:** add HTTP timeout and improve OAuth polling resilience ([cf7b9aa](https://github.com/jullury/akama/commit/cf7b9aa5f78728b95cb61a7fe22868c8c78dda6a))
* enhance agent prompts and add configuration management ([56d230a](https://github.com/jullury/akama/commit/56d230ab22d3fdecb921c8cac044a26a63527125))
* implement Go CLI replacing Docker/n8n setup ([1d7f51f](https://github.com/jullury/akama/commit/1d7f51fa9b46873a5c3b00731d992bafd999205d))
* migrate from n8n-based orchestration to Go CLI ([623daf2](https://github.com/jullury/akama/commit/623daf2f4c51011467e12896679bbe5f51166679))
