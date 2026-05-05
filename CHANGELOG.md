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
