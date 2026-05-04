package config

// Build-time OAuth App credentials for device flow authorization.
// These are injected at build time via -ldflags:
//   go build -ldflags "-X github.com/jullury/akama/internal/config.GitHubClientID=your_id \
//     -X github.com/jullury/akama/internal/config.GitHubClientSecret=your_secret \
//     -X github.com/jullury/akama/internal/config.GitLabClientID=your_id \
//     -X github.com/jullury/akama/internal/config.GitLabClientSecret=your_secret" -o akama .
//
// GitHub OAuth App: https://github.com/settings/developers (enable Device Flow)
// GitLab Application: https://gitlab.com/-/user_settings/applications (enable Device Authorization Grant)

var (
	GitHubClientID     string
	GitHubClientSecret string
	GitLabClientID     string
	GitLabClientSecret string
)
