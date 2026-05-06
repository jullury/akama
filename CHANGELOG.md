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
