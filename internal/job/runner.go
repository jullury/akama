package job

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/git"
	"github.com/jullury/akama/internal/knowledge"
	"github.com/jullury/akama/internal/metrics"
	"github.com/jullury/akama/internal/provider"
	"github.com/jullury/akama/internal/storage"
)

var wg sync.WaitGroup

var (
	cancelsMu sync.Mutex
	cancels   = make(map[int64]context.CancelFunc)
)

var (
	pkgDB       *sql.DB
	pkgBot      *tgbotapi.BotAPI
	pkgAgentCfg *agent.Config
	pkgWorkDir  string
	sem         chan struct{}
)

var pkgQuietStart int
var pkgQuietEnd   int
var pkgOllamaURL  string

// InitScheduler sets package-level state used by Run and RunGrouped.
// Must be called once before any jobs are started.
func InitScheduler(db *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config, workspaceDir string, maxConcurrent, quietStart, quietEnd int, ollamaURL string) {
	pkgDB = db
	pkgBot = bot
	pkgAgentCfg = agentCfg
	pkgWorkDir = workspaceDir
	if maxConcurrent > 0 {
		sem = make(chan struct{}, maxConcurrent)
	}
	pkgQuietStart = quietStart
	pkgQuietEnd = quietEnd
	pkgOllamaURL = ollamaURL
}

func isQuietHours() bool {
	if pkgQuietStart == pkgQuietEnd {
		return false
	}
	h := time.Now().Hour()
	if pkgQuietStart < pkgQuietEnd {
		return h >= pkgQuietStart && h < pkgQuietEnd
	}
	return h >= pkgQuietStart || h < pkgQuietEnd
}

func registerCancel(jobID int64, cancel context.CancelFunc) {
	cancelsMu.Lock()
	cancels[jobID] = cancel
	cancelsMu.Unlock()
}

func deregisterCancel(jobID int64) {
	cancelsMu.Lock()
	delete(cancels, jobID)
	cancelsMu.Unlock()
}

// CancelJob cancels a running job by jobID. Returns true if the job was found.
func CancelJob(jobID int64) bool {
	cancelsMu.Lock()
	cancel, ok := cancels[jobID]
	if ok {
		delete(cancels, jobID)
	}
	cancelsMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func Run(ctx context.Context, jobID int64) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		if sem != nil {
			sem <- struct{}{}
		}
		defer func() {
			if sem != nil {
				<-sem
			}
			maybeStartNextPending(ctx)
		}()
		jobCtx, cancel := context.WithCancel(ctx)
		registerCancel(jobID, cancel)
		defer deregisterCancel(jobID)
		defer cancel()
		runJob(jobCtx, jobID, pkgDB, pkgBot, pkgAgentCfg, pkgWorkDir)
	}()
}

func maybeStartNextPending(ctx context.Context) {
	if pkgDB == nil {
		return
	}
	j, err := storage.GetOldestPendingJob(pkgDB)
	if err != nil || j == nil {
		return
	}
	Run(ctx, j.ID)
}

func RunGrouped(ctx context.Context, groupID string, jobIDs []int64) {
	groupCtx, cancel := context.WithCancel(ctx)
	for _, jobID := range jobIDs {
		registerCancel(jobID, cancel)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, jobID := range jobIDs {
			deregisterCancel(jobID)
		}
		defer cancel()
		runGrouped(groupCtx, groupID, pkgDB, pkgBot, pkgAgentCfg, pkgWorkDir)
	}()
}

func runGrouped(ctx context.Context, groupID string, jobsDB *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config, workspaceDir string) {
	jobs, err := storage.FindJobsByGroupID(jobsDB, groupID)
	if err != nil || len(jobs) == 0 {
		log.Printf("runGrouped: no jobs found for group %s: %v", groupID, err)
		return
	}

	chatID := jobs[0].ChatID
	issueTitle := jobs[0].IssueTitle
	issueURL := jobs[0].IssueURL
	issueBody := jobs[0].IssueBody

	groupWorkspace := filepath.Join(workspaceDir, "multi", groupID)

	// Load user config and apply to all jobs
	userCfg, _ := storage.GetUserConfig(jobsDB, chatID)
	gitName, gitEmail := "", ""
	agentName, agentModel := jobs[0].Agent, jobs[0].AgentModel
	if userCfg != nil {
		gitName = userCfg.GitName
		gitEmail = userCfg.GitEmail
		if userCfg.Agent != "" {
			agentName = userCfg.Agent
		}
		if userCfg.AgentModel != "" {
			agentModel = userCfg.AgentModel
		}
	}

	// Refresh tokens from connections and set workspace for all jobs
	for _, j := range jobs {
		if conn, err := storage.FindConnectionByRepo(jobsDB, j.ChatID, j.RepoURL); err == nil && conn != nil {
			if conn.GitToken != j.GitToken {
				j.GitToken = conn.GitToken
				storage.UpdateJobToken(jobsDB, j.ID, conn.GitToken)
			}
		}
		storage.SetJobRunning(jobsDB, j.ID, groupWorkspace)
	}

	notify(bot, chatID, fmt.Sprintf("🔍 Cloning %d repositories for: %s...", len(jobs), issueTitle))

	// Clone all repos into subdirectories of the shared workspace
	clonePaths := make(map[int64]string)
	cloneFailedJobs := make(map[int64]bool)
	for _, j := range jobs {
		owner, repo, _ := git.OwnerRepo(j.RepoURL)
		dirName := owner + "-" + repo
		clonePath := filepath.Join(groupWorkspace, dirName)
		clonePaths[j.ID] = clonePath

		if err := withRetry(ctx, "git clone", 3, func() error {
			return git.Clone(j.RepoURL, j.GitToken, clonePath, j.DefaultBranch)
		}); err != nil {
			failGroupedJob(jobsDB, bot, j, fmt.Sprintf("git clone: %v", err))
			cloneFailedJobs[j.ID] = true
			continue
		}

		if err := chmodWorkspace(clonePath); err != nil {
			log.Printf("chmod workspace: %v", err)
		}

		if err := setupMise(clonePath); err != nil {
			log.Printf("mise install: %v", err)
		}
	}

	if len(cloneFailedJobs) > 0 {
		// Only cleanup if ALL clones failed
		if len(cloneFailedJobs) == len(jobs) {
			cleanupGroupWorkspace(jobsDB, groupID, groupWorkspace)
		}
	}

	// Build multi-repo prompt
	repoList := make([]string, 0, len(jobs))
	for _, j := range jobs {
		owner, repo, _ := git.OwnerRepo(j.RepoURL)
		repoList = append(repoList, fmt.Sprintf("  - %s/%s (%s)", owner, repo, j.Provider))
	}

	truncated := issueBody
	if len(issueBody) > 50000 {
		truncated = issueBody[:50000]
	}

	var knowledgePath string
	if pkgOllamaURL != "" {
		similarJobs, err := knowledge.FindSimilar(ctx, pkgDB, pkgOllamaURL,
			issueTitle+"\n"+issueBody, 3)
		if err != nil {
			log.Printf("knowledge lookup for group %s: %v", groupID, err)
		}
		if len(similarJobs) > 0 {
			knowledgePath, _ = knowledge.WriteKnowledgeFile(groupWorkspace, similarJobs)
		}
	}

	prompt := fmt.Sprintf(`You are a developer fixing an issue across %d repositories.

The workspace contains the following repositories as subdirectories:
%s

All repository code may be interdependent. Process all repositories at once and make changes across them as needed.

Issue Title: %s
Issue URL:   %s
Description:
%s

Implement a complete fix across all repositories. Make all necessary code changes.
Do NOT create pull requests or push branches — that is handled separately.
Do NOT mention AI, bots, automation, or any tool in code comments.
Write as a human developer would.
`, len(jobs), strings.Join(repoList, "\n"), issueTitle, issueURL, truncated)

	if knowledgePath != "" {
		prompt += fmt.Sprintf("\nPrior art from similar resolved issues is available in %s — read it before implementing.\n", filepath.Base(knowledgePath))
	}

	promptPath, err := agent.WritePrompt(groupWorkspace, agentName, prompt)
	if err != nil {
		for _, j := range jobs {
			failGroupedJob(jobsDB, bot, j, fmt.Sprintf("write prompt: %v", err))
		}
		return
	}

	notify(bot, chatID, fmt.Sprintf("🤖 Running AI agent across %d repositories for: %s", len(jobs), issueTitle))

	heartbeatStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		elapsed := 0
		for {
			select {
			case <-heartbeatStop:
				return
			case <-ticker.C:
				elapsed += 5
				notify(bot, chatID, fmt.Sprintf("⏳ Agent still working... (%d min elapsed across %d repos)", elapsed, len(jobs)))
			}
		}
	}()

	var rawOutput string
	agentErr := withRetry(ctx, "agent run", 3, func() error {
		var e error
		rawOutput, e = agent.Run(ctx, agentName, agentModel, groupWorkspace, promptPath, agentCfg)
		return e
	})
	close(heartbeatStop)

	if agentErr != nil {
		for _, j := range jobs {
			failGroupedJob(jobsDB, bot, j, fmt.Sprintf("agent run: %v", agentErr))
		}
		return
	}
	// Store agent output on the first job
	if len(jobs) > 0 {
		storage.SetJobAgentOutput(jobsDB, jobs[0].ID, rawOutput)
	}

	agentText := agent.ParseOutput(agentName, rawOutput)

	// Check for question
	if agent.IsQuestion(agentText) {
		if len(jobs) > 0 {
			storage.SetJobAwaitingInput(jobsDB, jobs[0].ID, agentText)
		}
		storage.SetConversationState(jobsDB, chatID, "telegram", "await_agent_input",
			map[string]interface{}{"job_id": float64(jobs[0].ID)})
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
			"🤔 Agent needs your input across %d repositories:\n\n%s\n\nJust reply with your answer.",
			len(jobs), agent.ExtractQuestion(agentText),
		))
		sent, _ := bot.Send(msg)
		if sent.MessageID != 0 && len(jobs) > 0 {
			storage.SetJobNotifMsgID(jobsDB, jobs[0].ID, int64(sent.MessageID))
		}
		return
	}

	if agentText != "" {
		notifyChunked(bot, chatID, "📋 Agent output across all repositories:", agentText)
	}

	// Process each repo: commit, push, create PR
	var prURLs []string
	for _, j := range jobs {
		if cloneFailedJobs[j.ID] {
			continue
		}
		owner, repo, _ := git.OwnerRepo(j.RepoURL)
		repoName := owner + "/" + repo
		clonePath := clonePaths[j.ID]

		notify(bot, chatID, fmt.Sprintf("📦 [%s] %s — committing and pushing changes...", j.Provider, repoName))

		commitMsg, prBody := agent.GenerateSummary(ctx, agentName, agentModel, clonePath, issueURL, agentCfg)
		branchName := agent.BranchFromCommit(commitMsg, fmt.Sprintf("fix/issue-%s", j.IssueID))

		if err := git.Commit(clonePath, branchName, j.GitToken, gitName, gitEmail, commitMsg); err != nil {
			failGroupedJob(jobsDB, bot, j, fmt.Sprintf("git commit: %v", err))
			continue
		}
		if err := withRetry(ctx, "git push", 3, func() error {
			return git.Push(clonePath, branchName, j.GitToken)
		}); err != nil {
			failGroupedJob(jobsDB, bot, j, fmt.Sprintf("git push: %v", err))
			continue
		}

		diffStat := git.DiffStat(clonePath)

		notify(bot, chatID, fmt.Sprintf("🔗 [%s] %s — creating pull request...", j.Provider, repoName))
		var prURL string
		if err := withRetry(ctx, "create PR", 3, func() error {
			var e error
			switch j.Provider {
			case "github":
				prURL, e = provider.CreateGitHubPR(j.RepoURL, j.GitToken, j.IssueTitle, branchName, j.DefaultBranch, prBody)
			case "gitlab":
				prURL, e = provider.CreateGitLabMR(j.RepoURL, j.GitToken, j.IssueTitle, branchName, j.DefaultBranch, prBody)
			}
			if e != nil && provider.IsPRAlreadyExists(e) {
				prURL, e = provider.FindExistingPR(j.RepoURL, j.GitToken, branchName, j.Provider)
			}
			return e
		}); err != nil {
			failGroupedJob(jobsDB, bot, j, fmt.Sprintf("create PR: %v", err))
			continue
		}

		storage.SetJobPRCreated(jobsDB, j.ID, branchName, prURL)
		prURLs = append(prURLs, fmt.Sprintf("[%s] %s — %s", j.Provider, repoName, prURL))

		// Embed completed job asynchronously
		go func(jobRef storage.Job) {
			if pkgOllamaURL != "" {
				knowledge.EmbedJob(context.Background(), pkgDB, pkgOllamaURL, jobRef)
			}
		}(*j)

		if diffStat != "" {
			notify(bot, chatID, fmt.Sprintf("[%s] %s diff:\n%s", j.Provider, repoName, diffStat))
		}

		// Poll CI for this repo
		go pollCI(ctx, j, branchName, bot)
	}

	// Send consolidated notification
	if len(prURLs) > 0 {
		msgText := "PRs ready across all repositories:\n" + strings.Join(prURLs, "\n")
		msgText += fmt.Sprintf("\n\nReply for follow-up or /done %d", jobs[0].ID)
		msg := tgbotapi.NewMessage(chatID, msgText)
		sent, _ := bot.Send(msg)
		if sent.MessageID != 0 && len(jobs) > 0 {
			storage.SetJobNotifMsgID(jobsDB, jobs[0].ID, int64(sent.MessageID))
		}
	}

	storage.SetConversationState(jobsDB, chatID, "telegram", "await_agent_input",
		map[string]interface{}{"job_id": float64(jobs[0].ID)})

	os.Remove(promptPath)
	if knowledgePath != "" {
		os.Remove(knowledgePath)
	}
}

func setupMise(dir string) error {
	cmd := exec.Command("mise", "install")
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mise install: %w", err)
	}
	return nil
}

func runJob(ctx context.Context, jobID int64, jobsDB *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config, workspaceDir string) {
	metrics.Global.Started.Add(1)
	metrics.Global.Active.Add(1)
	start := time.Now()
	defer func() {
		metrics.Global.Active.Add(-1)
		metrics.Global.TotalDurationMs.Add(time.Since(start).Milliseconds())
	}()

	j, err := storage.GetJob(jobsDB, jobID)
	if err != nil {
		log.Printf("get job %d: %v", jobID, err)
		return
	}

	// Refresh git token from connection so retried jobs pick up a refreshed token.
	var connAgent, connAgentModel string
	if conn, err := storage.FindConnectionByRepo(jobsDB, j.ChatID, j.RepoURL); err == nil && conn != nil {
		if conn.GitToken != j.GitToken {
			log.Printf("[runJob] Refreshing token from connection for job %d", jobID)
			j.GitToken = conn.GitToken
			if err := storage.UpdateJobToken(jobsDB, jobID, conn.GitToken); err != nil {
				log.Printf("[runJob] Failed to update job token: %v", err)
			}
		}
		connAgent = conn.Agent
		connAgentModel = conn.AgentModel
	}

	userCfg, err := storage.GetUserConfig(jobsDB, j.ChatID)
	if err != nil {
		log.Printf("get user config: %v", err)
	}
	gitName, gitEmail := "", ""
	if userCfg != nil {
		gitName = userCfg.GitName
		gitEmail = userCfg.GitEmail
		if userCfg.Agent != "" {
			j.Agent = userCfg.Agent
		}
		if userCfg.AgentModel != "" {
			j.AgentModel = userCfg.AgentModel
		}
	}
	if connAgent != "" {
		j.Agent = connAgent
	}
	if connAgentModel != "" {
		j.AgentModel = connAgentModel
	}

	repoPath, _ := url.Parse(j.RepoURL)
	parts := strings.Split(strings.Trim(repoPath.Path, "/"), "/")
	var workspacePath string
	if len(parts) >= 2 {
		workspacePath = filepath.Join(workspaceDir, j.Provider, parts[0], parts[1], j.IssueID)
	} else {
		workspacePath = filepath.Join(workspaceDir, fmt.Sprintf("%d", jobID))
	}

	owner, repo, _ := git.OwnerRepo(j.RepoURL)
	repoName := owner + "/" + repo

	if err := storage.SetJobRunning(jobsDB, jobID, workspacePath); err != nil {
		log.Printf("set running: %v", err)
		return
	}

	notify(bot, j.ChatID, fmt.Sprintf("🔍 [%s] %s — preparing workspace for: %s...", j.Provider, repoName, j.IssueTitle))

	_, statErr := os.Stat(filepath.Join(workspacePath, ".git"))
	if statErr != nil {
		os.MkdirAll(workspacePath, 0755)
		if err := withRetry(ctx, "git clone", 3, func() error {
			return git.Clone(j.RepoURL, j.GitToken, workspacePath, j.DefaultBranch)
		}); err != nil {
			metrics.Global.Failed.Add(1)
			failJob(jobsDB, bot, j, fmt.Sprintf("git clone: %v", err), workspacePath)
			return
		}
	} else {
		log.Printf("[runJob %d] Workspace exists at %s, skipping clone", jobID, workspacePath)
	}

	// Worker containers run as non-root (uid 1000); make the workspace writable.
	if err := chmodWorkspace(workspacePath); err != nil {
		log.Printf("chmod workspace: %v", err)
	}

	if err := setupMise(workspacePath); err != nil {
		log.Printf("mise install: %v", err)
	}

	notify(bot, j.ChatID, fmt.Sprintf("🤖 [%s] %s — running AI agent on: %s", j.Provider, repoName, j.IssueTitle))

	var knowledgePath string
	if pkgOllamaURL != "" {
		similarJobs, err := knowledge.FindSimilar(ctx, pkgDB, pkgOllamaURL,
			j.IssueTitle+"\n"+j.IssueBody, 3)
		if err != nil {
			log.Printf("knowledge lookup for job %d: %v", jobID, err)
		}
		if len(similarJobs) > 0 {
			knowledgePath, _ = knowledge.WriteKnowledgeFile(workspacePath, similarJobs)
		}
	}

	prompt := agent.BuildPrompt(j.IssueTitle, j.IssueURL, j.IssueBody, filepath.Base(knowledgePath))
	promptPath, err := agent.WritePrompt(workspacePath, j.Agent, prompt)
	if err != nil {
		metrics.Global.Failed.Add(1)
		failJob(jobsDB, bot, j, fmt.Sprintf("write prompt: %v", err), workspacePath)
		return
	}

	heartbeatStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		elapsed := 0
		for {
			select {
			case <-heartbeatStop:
				return
			case <-ticker.C:
				elapsed += 5
				notify(bot, j.ChatID, fmt.Sprintf("⏳ [%s] Agent still working... (%d min elapsed)", repoName, elapsed))
			}
		}
	}()

	var rawOutput string
	agentErr := withRetry(ctx, "agent run", 3, func() error {
		var e error
		rawOutput, e = agent.Run(ctx, j.Agent, j.AgentModel, workspacePath, promptPath, agentCfg)
		return e
	})
	close(heartbeatStop)

	if agentErr != nil {
		metrics.Global.Failed.Add(1)
		failJob(jobsDB, bot, j, fmt.Sprintf("agent run: %v", agentErr), workspacePath)
		return
	}
	storage.SetJobAgentOutput(jobsDB, jobID, rawOutput)

	agentText := agent.ParseOutput(j.Agent, rawOutput)

	if agent.IsQuestion(agentText) {
		if err := storage.SetJobAwaitingInput(jobsDB, jobID, agentText); err != nil {
			log.Printf("set awaiting_input: %v", err)
		}
		storage.SetConversationState(jobsDB, j.ChatID, "telegram", "await_agent_input",
			map[string]interface{}{"job_id": jobID})
		msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf(
			"🤔 [%s] %s — agent needs your input:\n\n%s\n\nJust reply with your answer.",
			j.Provider, repoName, agent.ExtractQuestion(agentText),
		))
		sent, _ := bot.Send(msg)
		if sent.MessageID != 0 {
			storage.SetJobNotifMsgID(jobsDB, jobID, int64(sent.MessageID))
		}
		return
	}

	if agentText != "" {
		notifyChunked(bot, j.ChatID, fmt.Sprintf("📋 [%s] Agent output:", j.Provider), agentText)
	}

	notify(bot, j.ChatID, fmt.Sprintf("📦 [%s] %s — committing and pushing changes...", j.Provider, repoName))

	commitMsg, prBody := agent.GenerateSummary(ctx, j.Agent, j.AgentModel, workspacePath, j.IssueURL, agentCfg)
	branchName := agent.BranchFromCommit(commitMsg, fmt.Sprintf("fix/issue-%s", j.IssueID))
	if err := git.Commit(workspacePath, branchName, j.GitToken, gitName, gitEmail, commitMsg); err != nil {
		metrics.Global.Failed.Add(1)
		failJob(jobsDB, bot, j, fmt.Sprintf("git commit: %v", err), workspacePath)
		return
	}
	if err := withRetry(ctx, "git push", 3, func() error {
		return git.Push(workspacePath, branchName, j.GitToken)
	}); err != nil {
		metrics.Global.Failed.Add(1)
		failJob(jobsDB, bot, j, fmt.Sprintf("git push: %v", err), workspacePath)
		return
	}

	diffStat := git.DiffStat(workspacePath)

	notify(bot, j.ChatID, fmt.Sprintf("🔗 [%s] %s — creating pull request...", j.Provider, repoName))
	var prURL string
	if err := withRetry(ctx, "create PR", 3, func() error {
		var e error
		switch j.Provider {
		case "github":
			prURL, e = provider.CreateGitHubPR(j.RepoURL, j.GitToken, j.IssueTitle, branchName, j.DefaultBranch, prBody)
		case "gitlab":
			prURL, e = provider.CreateGitLabMR(j.RepoURL, j.GitToken, j.IssueTitle, branchName, j.DefaultBranch, prBody)
		}
		if e != nil && provider.IsPRAlreadyExists(e) {
			prURL, e = provider.FindExistingPR(j.RepoURL, j.GitToken, branchName, j.Provider)
		}
		return e
	}); err != nil {
		metrics.Global.Failed.Add(1)
		failJob(jobsDB, bot, j, fmt.Sprintf("create PR: %v", err), workspacePath)
		return
	}

	metrics.Global.Succeeded.Add(1)

	if err := storage.SetJobPRCreated(jobsDB, jobID, branchName, prURL); err != nil {
		log.Printf("set pr_created: %v", err)
	}

	msgText := fmt.Sprintf("[%s] PR ready — %s", j.Provider, prURL)
	if diffStat != "" {
		msgText += "\n\n" + diffStat
	}
	msgText += fmt.Sprintf("\n\nReply for follow-up or /done %d", jobID)

	if !isQuietHours() {
		msg := tgbotapi.NewMessage(j.ChatID, msgText)
		sent, _ := bot.Send(msg)
		if sent.MessageID != 0 {
			storage.SetJobNotifMsgID(jobsDB, jobID, int64(sent.MessageID))
		}
	}

	storage.SetConversationState(jobsDB, j.ChatID, "telegram", "await_agent_input",
		map[string]interface{}{"job_id": jobID})

	go pollCI(ctx, j, branchName, bot)

	os.Remove(promptPath)
	if knowledgePath != "" {
		os.Remove(knowledgePath)
	}

	go func() {
		if pkgOllamaURL != "" {
			updated, err := storage.GetJob(pkgDB, jobID)
			if err == nil {
				knowledge.EmbedJob(context.Background(), pkgDB, pkgOllamaURL, *updated)
			}
		}
	}()
}

func pollCI(ctx context.Context, j *storage.Job, branch string, bot *tgbotapi.BotAPI) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	deadline := time.NewTimer(20 * time.Minute)
	defer deadline.Stop()

	noneCount := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		case <-ticker.C:
			status, err := provider.GetCIStatus(j.RepoURL, j.GitToken, branch, j.Provider)
			if err != nil {
				log.Printf("CI poll job %d: %v", j.ID, err)
				continue
			}
			switch status.State {
			case "none":
				noneCount++
				if noneCount >= 3 {
					return
				}
			case "pending":
				noneCount = 0
			case "success":
				notify(bot, j.ChatID, fmt.Sprintf("✅ [%s] CI passed — %s", j.Provider, j.PRURL))
				return
			case "failure":
				msg := fmt.Sprintf("❌ [%s] CI failed for job #%d", j.Provider, j.ID)
				if status.URL != "" {
					msg += "\n" + status.URL
				}
				notify(bot, j.ChatID, msg)
				return
			}
		}
	}
}

func notify(bot *tgbotapi.BotAPI, chatID int64, text string) {
	bot.Send(tgbotapi.NewMessage(chatID, text))
}

func notifyChunked(bot *tgbotapi.BotAPI, chatID int64, header, body string) {
	const maxLen = 4000
	prefix := header + "\n\n"
	remaining := strings.TrimSpace(body)
	first := true
	for remaining != "" {
		avail := maxLen
		if first {
			avail -= len(prefix)
		}
		var chunk string
		if len(remaining) <= avail {
			chunk = remaining
			remaining = ""
		} else {
			cutAt := avail
			if idx := strings.LastIndex(remaining[:cutAt], "\n"); idx > 0 {
				cutAt = idx
			}
			chunk = remaining[:cutAt]
			remaining = strings.TrimSpace(remaining[cutAt:])
		}
		text := chunk
		if first {
			text = prefix + chunk
			first = false
		}
		notify(bot, chatID, text)
	}
}

func failJob(jobsDB *sql.DB, bot *tgbotapi.BotAPI, j *storage.Job, errMsg, workspacePath string) {
	storage.SetJobFailed(jobsDB, j.ID, errMsg)
	if !isQuietHours() {
		err := fmt.Errorf("%s", errMsg)
		if provider.IsWorkflowScopeError(err) {
			msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf(
				"❌ Job %d failed: the changes include a GitHub Actions workflow file but your token lacks the `workflow` scope.\n\n"+
					"Use /connect to re-authenticate (the new token will include workflow permissions), then /retry %d.",
				j.ID, j.ID,
			))
			bot.Send(msg)
		} else if provider.IsAuthError(err) {
			msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf(
				"❌ Job %d failed: authentication error.\n\n"+
					"Your token for %s may have expired or been revoked.\n"+
					"Use /connect to refresh your token, then /retry %d to try again.",
				j.ID, j.Provider, j.ID,
			))
			bot.Send(msg)
		} else {
			msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("❌ Job %d failed: %s\n\nUse /logs %d to view details.", j.ID, errMsg, j.ID))
			bot.Send(msg)
		}
	}
	os.RemoveAll(workspacePath)
}

func failGroupedJob(jobsDB *sql.DB, bot *tgbotapi.BotAPI, j *storage.Job, errMsg string) {
	storage.SetJobFailed(jobsDB, j.ID, errMsg)
	if provider.IsAuthError(fmt.Errorf("%s", errMsg)) {
		msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf(
			"❌ Job %d failed: authentication error for %s.\n\n"+
				"Your token for %s may have expired or been revoked.\n"+
				"Use /connect to refresh your token.",
			j.ID, j.RepoURL, j.Provider,
		))
		bot.Send(msg)
	} else {
		msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("❌ Job %d (%s) failed: %s\n\nUse /logs %d to view details.", j.ID, j.RepoURL, errMsg, j.ID))
		bot.Send(msg)
	}
}

func cleanupGroupWorkspace(jobsDB *sql.DB, groupID, workspacePath string) {
	active, _ := storage.CountActiveJobsByGroupID(jobsDB, groupID)
	if active == 0 {
		os.RemoveAll(workspacePath)
	}
}

func WaitForJobs(timeoutSec int) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Duration(timeoutSec) * time.Second):
	}
}

// chmodWorkspace recursively opens workspace permissions so that non-root worker
// containers (uid 1000) can read and write files created by the root daemon.
func chmodWorkspace(path string) error {
	return filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return os.Chmod(p, 0777)
		}
		return os.Chmod(p, 0666)
	})
}
