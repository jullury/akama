package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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
var pkgQuietEnd int
var pkgOllamaURL string

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
	if ollamaURL == "" {
		log.Printf("[knowledge] Ollama URL not configured; knowledge base flow disabled")
	}
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

// runProfileWithHeartbeat runs a single profile phase while emitting a periodic
// "still working" notification so long-running phases don't look stuck.
func runProfileWithHeartbeat(ctx context.Context, bot *tgbotapi.BotAPI, chatID int64, providerName string, prof agent.Profile, agentName, model, workspacePath, prompt string, cfg *agent.Config) (string, error) {
	heartbeatStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatStop:
				return
			case <-ticker.C:
				notify(bot, chatID, fmt.Sprintf("⏳ [%s] %s still working...", providerName, prof.DisplayName()))
			}
		}
	}()
	defer close(heartbeatStop)
	return agent.RunProfileAgent(ctx, prof, agentName, model, workspacePath, prompt, cfg)
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

	// Refresh tokens from connections
	for _, j := range jobs {
		if conn, err := storage.FindConnectionByRepo(jobsDB, j.ChatID, j.RepoURL); err == nil && conn != nil {
			if conn.GitToken != j.GitToken {
				j.GitToken = conn.GitToken
				storage.UpdateJobToken(jobsDB, j.ID, conn.GitToken)
			}
		}
	}

	notify(bot, chatID, fmt.Sprintf("🔍 Cloning %d repositories for: %s...", len(jobs), issueTitle))

	// Each job's WorkspacePath is a subdirectory under the shared issue workspace.
	// The shared issue workspace is the parent directory.
	sharedWorkspace := filepath.Dir(jobs[0].WorkspacePath)

	// Clone each repo into its subdirectory under the shared issue workspace
	clonePaths := make(map[int64]string)
	cloneFailedJobs := make(map[int64]bool)
	for _, j := range jobs {
		clonePath := j.WorkspacePath
		if clonePath == "" {
			// Fallback: compute path from repo URL
			owner, repo, _ := git.OwnerRepo(j.RepoURL)
			clonePath = filepath.Join(sharedWorkspace, fmt.Sprintf("%s-%s", owner, repo))
			storage.SetJobRunning(jobsDB, j.ID, clonePath)
		} else {
			storage.SetJobRunning(jobsDB, j.ID, clonePath)
		}
		clonePaths[j.ID] = clonePath

		if _, err := os.Stat(filepath.Join(clonePath, ".git")); err == nil {
			log.Printf("[runGrouped] Workspace exists at %s, pulling latest for job %d", clonePath, j.ID)
			if err := withRetry(ctx, "git pull", 3, func() error {
				return git.Pull(clonePath, j.GitToken, j.DefaultBranch)
			}); err != nil {
				log.Printf("[runGrouped] pull failed (%v); re-cloning", err)
				os.RemoveAll(clonePath)
				os.MkdirAll(clonePath, 0755)
				if err := withRetry(ctx, "git clone", 3, func() error {
					return git.Clone(j.RepoURL, j.GitToken, clonePath, j.DefaultBranch)
				}); err != nil {
					failGroupedJob(jobsDB, bot, j, fmt.Sprintf("git clone: %v", err))
					cloneFailedJobs[j.ID] = true
					continue
				}
			}
		} else {
			os.MkdirAll(clonePath, 0755)
			if err := withRetry(ctx, "git clone", 3, func() error {
				return git.Clone(j.RepoURL, j.GitToken, clonePath, j.DefaultBranch)
			}); err != nil {
				failGroupedJob(jobsDB, bot, j, fmt.Sprintf("git clone: %v", err))
				cloneFailedJobs[j.ID] = true
				continue
			}
		}

		if err := ChmodWorkspace(clonePath); err != nil {
			log.Printf("chmod workspace: %v", err)
		}

		if err := setupMise(clonePath); err != nil {
			log.Printf("mise install: %v", err)
		}
	}

	if len(cloneFailedJobs) > 0 && len(cloneFailedJobs) == len(jobs) {
		// All clones failed — clean up the shared workspace
		os.RemoveAll(sharedWorkspace)
	}

	// Build multi-repo prompt — repos are subdirectories of CWD
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
	var similarJobIDs []int64
	if pkgOllamaURL != "" {
		queryBody := issueBody
		if len(queryBody) > 2000 {
			queryBody = queryBody[:2000]
		}
		// For grouped jobs, search across all repos in the group
		// Use the first job's repo as primary context
		primaryRepoURL := ""
		if len(jobs) > 0 {
			primaryRepoURL = jobs[0].RepoURL
		}
		similarJobs, err := knowledge.FindSimilarForRepo(ctx, pkgDB, pkgOllamaURL,
			issueTitle+"\n"+queryBody, primaryRepoURL, 5)
		if err != nil {
			log.Printf("knowledge lookup for group %s: %v", groupID, err)
		}
		if len(similarJobs) > 0 {
			knowledgePath, _ = knowledge.WriteKnowledgeFile(sharedWorkspace, similarJobs)
			// Track similar job IDs for feedback
			for _, sj := range similarJobs {
				similarJobIDs = append(similarJobIDs, sj.ID)
			}
			// Track knowledge usage for primary job
			if len(jobs) > 0 {
				knowledge.TrackUsage(ctx, pkgDB, jobs[0].ID, knowledgePath, similarJobIDs)
			}
		}
	}

	// Run pre-implementation agent profiles (spec, plan, task, analyze).
	// Resumable: skip phases already present in profile_outputs.
	profileOutputs := agent.ParseProfileOutputs(func() string {
		if len(jobs) > 0 {
			jj, _ := storage.GetJob(jobsDB, jobs[0].ID)
			if jj != nil {
				return jj.ProfileOutputs
			}
		}
		return ""
	}())
	priorOutputs := ""
	for _, p := range agent.ProfileOrder {
		out, ok := profileOutputs[string(p)]
		if !ok || out == "" {
			break
		}
		priorOutputs += fmt.Sprintf("\n## %s\n%s\n", p, out)
	}

	var promptPath string
	var rawOutput string
	agentText := profileOutputs[string(agent.ProfileImplement)]

	// Spec profile (may ask clarifying questions).
	if _, ok := profileOutputs[string(agent.ProfileSpec)]; !ok {
		notify(bot, chatID, fmt.Sprintf("🤖 [%d repos] running: %s", len(jobs), agent.ProfileSpec.DisplayName()))
		specOut, err := runProfileWithHeartbeat(ctx, bot, chatID, "group", agent.ProfileSpec, agentName, agentModel, sharedWorkspace, agent.ProfilePrompt(agent.ProfileSpec, issueTitle, issueBody, sharedWorkspace, priorOutputs), agentCfg)
		if err != nil {
			log.Printf("[runGrouped] profile %s failed: %v", agent.ProfileSpec, err)
			for _, j := range jobs {
				failGroupedJob(jobsDB, bot, j, fmt.Sprintf("Spec phase failed: %v", err))
			}
			return
		}
		profileOutputs[string(agent.ProfileSpec)] = specOut
		if specOut != "" {
			priorOutputs += fmt.Sprintf("\n## %s\n%s\n", agent.ProfileSpec, specOut)
		}
		if len(jobs) > 0 {
			storage.SetJobProfileOutputs(jobsDB, jobs[0].ID, profileOutputs.Marshal())
		}

		if agent.IsQuestion(specOut) {
			qc := 0
			if len(jobs) > 0 {
				qc, _ = storage.GetQuestionCount(jobsDB, jobs[0].ID)
			}
			if qc < 3 {
				log.Printf("[runGrouped] spec profile asked a clarification question")
				if len(jobs) > 0 {
					storage.SetJobAwaitingInput(jobsDB, jobs[0].ID, specOut)
					storage.SetConversationState(jobsDB, chatID, "telegram", "await_agent_input",
						map[string]interface{}{"job_id": float64(jobs[0].ID), "grouped": true, "group_id": jobs[0].GroupID})
					msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
						"🤔 Spec needs clarification across %d repositories:\n\n%s\n\nJust reply with your answer.",
						len(jobs), agent.ExtractQuestion(specOut),
					))
					sent, _ := bot.Send(msg)
					if sent.MessageID != 0 && len(jobs) > 0 {
						storage.SetJobNotifMsgID(jobsDB, jobs[0].ID, int64(sent.MessageID))
					}
				}
				return
			}
			log.Printf("[runGrouped] spec asked a question but question limit reached; continuing")
		}

		// Post the spec as a comment on the issue before relaying to the next profile.
		if !agent.IsQuestion(specOut) && specOut != "" && len(jobs) > 0 {
			comment := fmt.Sprintf("## Technical Specification\n\n%s", specOut)
			if err := provider.PostIssueComment(jobs[0].Provider, issueURL, jobs[0].GitToken, comment); err != nil {
				log.Printf("[runGrouped] post spec comment: %v", err)
			} else {
				notify(bot, chatID, "📝 Spec posted as comment on the issue.")
			}
		}
	}

	// Plan profile (run if not already completed).
	if _, ok := profileOutputs[string(agent.ProfilePlan)]; !ok {
		notify(bot, chatID, fmt.Sprintf("🤖 [%d repos] running: %s", len(jobs), agent.ProfilePlan.DisplayName()))
		planOut, err := runProfileWithHeartbeat(ctx, bot, chatID, "group", agent.ProfilePlan, agentName, agentModel, sharedWorkspace, agent.ProfilePrompt(agent.ProfilePlan, issueTitle, issueBody, sharedWorkspace, priorOutputs), agentCfg)
		if err != nil {
			log.Printf("[runGrouped] profile %s failed: %v", agent.ProfilePlan, err)
			for _, j := range jobs {
				failGroupedJob(jobsDB, bot, j, fmt.Sprintf("Plan phase failed: %v", err))
			}
			return
		}
		profileOutputs[string(agent.ProfilePlan)] = planOut
		if planOut != "" {
			priorOutputs += fmt.Sprintf("\n## %s\n%s\n", agent.ProfilePlan, planOut)
			notify(bot, chatID, fmt.Sprintf("📋 Plan posted as a comment on the issue: %s", issueURL))
		}
		if len(jobs) > 0 {
			storage.SetJobProfileOutputs(jobsDB, jobs[0].ID, profileOutputs.Marshal())
		}
	}

	// Plan review gate (grouped).
	if len(jobs) > 0 {
		jj, _ := storage.GetJob(jobsDB, jobs[0].ID)
		if jj != nil && jj.Status != "plan_confirmed" {
			planOut := profileOutputs[string(agent.ProfilePlan)]
			if planOut != "" {
				comment := fmt.Sprintf("## Implementation Plan\n\n%s", planOut)
				if err := provider.PostIssueComment(jobs[0].Provider, issueURL, jobs[0].GitToken, comment); err != nil {
					log.Printf("[runGrouped] post plan comment: %v", err)
				}
			}
			notify(bot, chatID, fmt.Sprintf("📋 Plan ready across %d repositories. Confirm to proceed, or reply with feedback.\nAuto-continues in 5 minutes.", len(jobs)))
			kb := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("✅ Confirm", "plan:confirm"),
					tgbotapi.NewInlineKeyboardButtonData("✏️ Request changes", "plan:revise"),
				),
			)
			msg := tgbotapi.NewMessage(chatID, "Choose an action:")
			msg.ReplyMarkup = kb
			bot.Send(msg)
			storage.SetJobStatus(jobsDB, jobs[0].ID, "awaiting_plan_approval")
			storage.SetConversationState(jobsDB, chatID, "telegram", "await_plan_approval",
				map[string]interface{}{"job_id": float64(jobs[0].ID), "grouped": true, "group_id": jobs[0].GroupID})

			// Auto-continue after 5 minutes if the user does nothing.
			go func() {
				time.Sleep(5 * time.Minute)
				jj, err := storage.GetJob(jobsDB, jobs[0].ID)
				if err != nil || jj == nil || jj.Status != "awaiting_plan_approval" {
					return
				}
				log.Printf("[runGrouped] plan approval timed out; auto-continuing")
				storage.SetJobStatus(jobsDB, jobs[0].ID, "plan_confirmed")
				for _, j := range jobs {
					storage.SetJobStatus(jobsDB, j.ID, "plan_confirmed")
				}
				storage.ResetConversation(jobsDB, chatID, "telegram")
				RunGrouped(context.Background(), jobs[0].GroupID, func() []int64 {
					ids := make([]int64, 0, len(jobs))
					for _, j := range jobs {
						ids = append(ids, j.ID)
					}
					return ids
				}())
			}()

			// Stop here; continuation is triggered by the user or the timer above.
			return
		}
	}
	log.Printf("[runGrouped] plan review gate done; proceeding to task/analyze")
	notify(bot, chatID, "🤖 proceeding to task breakdown...")

	// Guard: do not proceed to tasks without a real plan. Retry plan a couple
	// of times if the output is missing/invalid.
	planOut := profileOutputs[string(agent.ProfilePlan)]
	retries := 0
	for !agent.IsValidPlan(planOut) && retries < 2 {
		retries++
		notify(bot, chatID, fmt.Sprintf("⚠️ Plan output invalid (attempt %d); re-running plan phase...", retries))
		newOut, err := runProfileWithHeartbeat(ctx, bot, chatID, "group", agent.ProfilePlan, agentName, agentModel, sharedWorkspace, agent.ProfilePrompt(agent.ProfilePlan, issueTitle, issueBody, sharedWorkspace, priorOutputs), agentCfg)
		if err != nil {
			log.Printf("[runGrouped] plan profile retry failed: %v", err)
			break
		}
		planOut = newOut
		profileOutputs[string(agent.ProfilePlan)] = planOut
		if planOut != "" {
			priorOutputs += fmt.Sprintf("\n## %s\n%s\n", agent.ProfilePlan, planOut)
		}
		if len(jobs) > 0 {
			storage.SetJobProfileOutputs(jobsDB, jobs[0].ID, profileOutputs.Marshal())
		}
	}
	if !agent.IsValidPlan(planOut) {
		for _, j := range jobs {
			failGroupedJob(jobsDB, bot, j, "plan profile produced no valid plan")
		}
		return
	}

	// Task and analyze (skip completed).
	for _, prof := range []agent.Profile{agent.ProfileTask, agent.ProfileAnalyze} {
		if _, ok := profileOutputs[string(prof)]; ok {
			continue
		}
		notify(bot, chatID, fmt.Sprintf("🤖 [%d repos] running: %s", len(jobs), prof.DisplayName()))
		out, err := runProfileWithHeartbeat(ctx, bot, chatID, "group", prof, agentName, agentModel, sharedWorkspace, agent.ProfilePrompt(prof, issueTitle, issueBody, sharedWorkspace, priorOutputs), agentCfg)
		if err != nil {
			log.Printf("[runGrouped] profile %s failed: %v", prof, err)
			out = fmt.Sprintf("phase %s failed: %v", prof, err)
		}
		profileOutputs[string(prof)] = out
		if out != "" {
			priorOutputs += fmt.Sprintf("\n## %s\n%s\n", prof, out)
		}
		if len(jobs) > 0 {
			storage.SetJobProfileOutputs(jobsDB, jobs[0].ID, profileOutputs.Marshal())
		}
	}

	// Implement phase (main multi-repo code-changing run).
	errQuestion := errors.New("agent question")
	runGroupedImplement := func() (string, error) {
		multiRepoPrompt := fmt.Sprintf(`You are a developer fixing an issue across %d repositories.

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
			multiRepoPrompt += fmt.Sprintf("\nPrior art from similar resolved issues is available in %s — read it before implementing.\n", filepath.Base(knowledgePath))
		}

		implementPrompt := agent.ProfilePrompt(agent.ProfileImplement, issueTitle, issueBody, sharedWorkspace, priorOutputs) + "\n" + multiRepoPrompt

		var err error
		promptPath, err = agent.WritePrompt(sharedWorkspace, agentName, implementPrompt)
		if err != nil {
			for _, j := range jobs {
				failGroupedJob(jobsDB, bot, j, fmt.Sprintf("write prompt: %v", err))
			}
			return "", fmt.Errorf("write prompt: %w", err)
		}

		notify(bot, chatID, fmt.Sprintf("🤖 Running AI agent across %d repositories for: %s", len(jobs), issueTitle))

		heartbeatStop := make(chan struct{})
		var agentProc *os.Process
		var agentProcMu sync.Mutex
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
					agentProcMu.Lock()
					proc := agentProc
					agentProcMu.Unlock()
					if proc != nil {
						if err := proc.Signal(syscall.Signal(0)); err != nil {
							notify(bot, chatID, fmt.Sprintf("⚠️ Agent process died after %d min — retrying", elapsed))
							return
						}
					}
					notify(bot, chatID, fmt.Sprintf("⏳ Agent still working... (%d min elapsed across %d repos)", elapsed, len(jobs)))
				}
			}
		}()

		agentErr := withRetry(ctx, "agent run", 3, func() error {
			var e error
			rawOutput, e = agent.RunDetached(ctx, agentName, agentModel, sharedWorkspace, promptPath, agentCfg, func(p *os.Process) {
				agentProcMu.Lock()
				agentProc = p
				agentProcMu.Unlock()
			})
			return e
		})
		close(heartbeatStop)

		if agentErr != nil {
			for _, j := range jobs {
				failGroupedJob(jobsDB, bot, j, fmt.Sprintf("agent run: %v", agentErr))
			}
			return "", fmt.Errorf("agent run: %v", agentErr)
		}
		// Store agent output on the first job
		if len(jobs) > 0 {
			storage.SetJobAgentOutput(jobsDB, jobs[0].ID, rawOutput)
		}

		agentText = agent.ParseOutput(agentName, rawOutput)

		profileOutputs[string(agent.ProfileImplement)] = agentText
		priorOutputs += "\n## implement\n" + agentText
		if len(jobs) > 0 {
			storage.SetJobProfileOutputs(jobsDB, jobs[0].ID, profileOutputs.Marshal())
		}

		// Check for question
		if agent.IsQuestion(agentText) {
			if len(jobs) > 0 {
				storage.SetJobAwaitingInput(jobsDB, jobs[0].ID, agentText)
			}
			storage.SetConversationState(jobsDB, chatID, "telegram", "await_agent_input",
				map[string]interface{}{"job_id": float64(jobs[0].ID), "grouped": true, "group_id": jobs[0].GroupID})
			msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
				"🤔 Agent needs your input across %d repositories:\n\n%s\n\nJust reply with your answer.",
				len(jobs), agent.ExtractQuestion(agentText),
			))
			sent, _ := bot.Send(msg)
			if sent.MessageID != 0 && len(jobs) > 0 {
				storage.SetJobNotifMsgID(jobsDB, jobs[0].ID, int64(sent.MessageID))
			}
			return agentText, errQuestion
		}

		if agentText != "" {
			notifyChunked(bot, chatID, "📋 Agent output across all repositories:", agentText)
		}
		return agentText, nil
	}

	// First pass: run implement if not already completed.
	if _, ok := profileOutputs[string(agent.ProfileImplement)]; !ok {
		at, err := runGroupedImplement()
		if err == errQuestion {
			return
		}
		if err != nil {
			return
		}
		agentText = at
	}

	// Converge phase.
	if _, ok := profileOutputs[string(agent.ProfileConverge)]; !ok {
		notify(bot, chatID, fmt.Sprintf("🤖 [%d repos] running: %s", len(jobs), agent.ProfileConverge.DisplayName()))
		convergeOut, _ := runProfileWithHeartbeat(ctx, bot, chatID, "group", agent.ProfileConverge, agentName, agentModel, sharedWorkspace, agent.ProfilePrompt(agent.ProfileConverge, issueTitle, issueBody, sharedWorkspace, priorOutputs), agentCfg)
		profileOutputs[string(agent.ProfileConverge)] = convergeOut
		priorOutputs += "\n## converge\n" + convergeOut
		if len(jobs) > 0 {
			storage.SetJobProfileOutputs(jobsDB, jobs[0].ID, profileOutputs.Marshal())
		}
	}

	// QA phase (after converge, before review).
	qaOutput := ""
	if v, ok := profileOutputs[string(agent.ProfileQA)]; ok {
		qaOutput = v
	}
	if _, ok := profileOutputs[string(agent.ProfileQA)]; !ok {
		notify(bot, chatID, fmt.Sprintf("🤖 [%d repos] running: %s", len(jobs), agent.ProfileQA.DisplayName()))
		var qaErr error
		qaOutput, qaErr = runProfileWithHeartbeat(ctx, bot, chatID, "group", agent.ProfileQA, agentName, agentModel, sharedWorkspace, agent.ProfilePrompt(agent.ProfileQA, issueTitle, issueBody, sharedWorkspace, priorOutputs), agentCfg)
		if qaErr != nil {
			log.Printf("[runGrouped] profile %s failed: %v", agent.ProfileQA, qaErr)
			for _, j := range jobs {
				failGroupedJob(jobsDB, bot, j, fmt.Sprintf("QA phase failed: %v", qaErr))
			}
			return
		}
		profileOutputs[string(agent.ProfileQA)] = qaOutput
		if qaOutput != "" {
			priorOutputs += "\n## qa\n" + qaOutput
		}
		if len(jobs) > 0 {
			storage.SetJobProfileOutputs(jobsDB, jobs[0].ID, profileOutputs.Marshal())
		}
	}

	// Fix-and-reverify loop on blocking QA defects.
	qaIterations := 0
	if v, ok := profileOutputs["qa_fix_iterations"]; ok && v != "" {
		fmt.Sscanf(v, "%d", &qaIterations)
	}
	for agent.IsQAFailure(qaOutput) && qaIterations < agent.QAMaxFixIterations {
		qaIterations++
		profileOutputs["qa_fix_iterations"] = fmt.Sprintf("%d", qaIterations)
		if len(jobs) > 0 {
			storage.SetJobProfileOutputs(jobsDB, jobs[0].ID, profileOutputs.Marshal())
		}

		notify(bot, chatID, fmt.Sprintf("🔁 [%d repos] QA found defects; re-running implement (iteration %d/%d)...", len(jobs), qaIterations, agent.QAMaxFixIterations))

		// Re-run implement phase via the shared closure (multi-repo RunDetached).
		_, err := runGroupedImplement()
		if err == errQuestion {
			return
		}
		if err != nil {
			return
		}

		// Re-run converge.
		notify(bot, chatID, fmt.Sprintf("🤖 [%d repos] running: %s", len(jobs), agent.ProfileConverge.DisplayName()))
		convergeOut, convErr := runProfileWithHeartbeat(ctx, bot, chatID, "group", agent.ProfileConverge, agentName, agentModel, sharedWorkspace, agent.ProfilePrompt(agent.ProfileConverge, issueTitle, issueBody, sharedWorkspace, priorOutputs), agentCfg)
		if convErr != nil {
			log.Printf("[runGrouped] profile %s failed: %v", agent.ProfileConverge, convErr)
			for _, j := range jobs {
				failGroupedJob(jobsDB, bot, j, fmt.Sprintf("Converge phase failed: %v", convErr))
			}
			return
		}
		profileOutputs[string(agent.ProfileConverge)] = convergeOut
		priorOutputs += "\n## converge\n" + convergeOut
		if len(jobs) > 0 {
			storage.SetJobProfileOutputs(jobsDB, jobs[0].ID, profileOutputs.Marshal())
		}

		// Re-run QA.
		notify(bot, chatID, fmt.Sprintf("🤖 [%d repos] running: %s", len(jobs), agent.ProfileQA.DisplayName()))
		var qaErr error
		qaOutput, qaErr = runProfileWithHeartbeat(ctx, bot, chatID, "group", agent.ProfileQA, agentName, agentModel, sharedWorkspace, agent.ProfilePrompt(agent.ProfileQA, issueTitle, issueBody, sharedWorkspace, priorOutputs), agentCfg)
		if qaErr != nil {
			log.Printf("[runGrouped] profile %s failed: %v", agent.ProfileQA, qaErr)
			for _, j := range jobs {
				failGroupedJob(jobsDB, bot, j, fmt.Sprintf("QA phase failed: %v", qaErr))
			}
			return
		}
		profileOutputs[string(agent.ProfileQA)] = qaOutput
		if qaOutput != "" {
			priorOutputs += "\n## qa\n" + qaOutput
		}
		if len(jobs) > 0 {
			storage.SetJobProfileOutputs(jobsDB, jobs[0].ID, profileOutputs.Marshal())
		}
	}

	if agent.IsQAFailure(qaOutput) {
		for _, j := range jobs {
			failGroupedJob(jobsDB, bot, j, "QA phase failed after fix iterations")
		}
		return
	}

	// Reviewer phase.
	reviewOutput := profileOutputs[string(agent.ProfileReviewer)]
	if _, ok := profileOutputs[string(agent.ProfileReviewer)]; !ok {
		notify(bot, chatID, fmt.Sprintf("🤖 [%d repos] running: %s", len(jobs), agent.ProfileReviewer.DisplayName()))
		reviewOutput, _ = runProfileWithHeartbeat(ctx, bot, chatID, "group", agent.ProfileReviewer, agentName, agentModel, sharedWorkspace, agent.ProfilePrompt(agent.ProfileReviewer, issueTitle, issueBody, sharedWorkspace, priorOutputs), agentCfg)
		profileOutputs[string(agent.ProfileReviewer)] = reviewOutput
		if len(jobs) > 0 {
			storage.SetJobProfileOutputs(jobsDB, jobs[0].ID, profileOutputs.Marshal())
		}
	}

	// Post reviewer output as a comment on the issue.
	if reviewOutput != "" && len(jobs) > 0 {
		if err := provider.PostIssueComment(jobs[0].Provider, issueURL, jobs[0].GitToken, "## Agent Review\n\n"+reviewOutput); err != nil {
			log.Printf("[runGrouped] post review comment: %v", err)
		}
	}
	// Post QA report as a comment on the issue.
	if qaOutput != "" && len(jobs) > 0 {
		if err := provider.PostIssueComment(jobs[0].Provider, issueURL, jobs[0].GitToken, "## QA Report\n\n"+qaOutput); err != nil {
			log.Printf("[runGrouped] post qa comment: %v", err)
		}
	} else if len(jobs) > 0 {
		notify(bot, chatID, "✅ QA passed — no defects found.")
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
			if pkgOllamaURL == "" {
				return
			}
			if err := knowledge.EmbedJob(context.Background(), pkgDB, pkgOllamaURL, jobRef); err != nil {
				log.Printf("[knowledge] embed job %d failed: %v", jobRef.ID, err)
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

func miseBinary() string {
	if path, err := exec.LookPath("mise"); err == nil {
		return path
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", "mise"),
		"/usr/local/bin/mise",
		"/opt/homebrew/bin/mise",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func setupMise(dir string) error {
	bin := miseBinary()
	if bin == "" {
		return fmt.Errorf("mise install: exec: \"mise\": executable file not found in $PATH")
	}
	cmd := exec.Command(bin, "install")
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

	owner, repo, _ := git.OwnerRepo(j.RepoURL)
	repoName := owner + "/" + repo

	// Prefer the workspace path already stored by the plan phase (set via
	// SetJobWorkspace during startPlanMode).  This avoids re-cloning a repo
	// that was already cloned for plan generation.
	workspacePath := j.WorkspacePath
	if workspacePath != "" {
		if _, err := os.Stat(filepath.Join(workspacePath, ".git")); err == nil {
			log.Printf("[runJob %d] Reusing plan-phase workspace at %s", jobID, workspacePath)
		} else {
			// Plan-phase workspace lost its .git (cleaned up or never cloned);
			// fall back to computing a fresh path.
			log.Printf("[runJob %d] Plan workspace %s has no .git, recomputing path", jobID, workspacePath)
			workspacePath = ""
		}
	}
	if workspacePath == "" {
		repoParsed, _ := url.Parse(j.RepoURL)
		parts := strings.Split(strings.Trim(repoParsed.Path, "/"), "/")
		if len(parts) >= 2 {
			workspacePath = filepath.Join(workspaceDir, j.Provider, parts[0], parts[1], j.IssueID)
		} else {
			workspacePath = filepath.Join(workspaceDir, fmt.Sprintf("%d", jobID))
		}
	}

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
		log.Printf("[runJob %d] Workspace exists at %s, pulling latest", jobID, workspacePath)
		notify(bot, j.ChatID, fmt.Sprintf("🔄 [%s] updating existing workspace...", j.Provider))
		if err := withRetry(ctx, "git pull", 3, func() error {
			return git.Pull(workspacePath, j.GitToken, j.DefaultBranch)
		}); err != nil {
			log.Printf("[runJob %d] pull failed (%v); re-cloning", jobID, err)
			if err := withRetry(ctx, "git clone", 3, func() error {
				return git.Clone(j.RepoURL, j.GitToken, workspacePath, j.DefaultBranch)
			}); err != nil {
				metrics.Global.Failed.Add(1)
				failJob(jobsDB, bot, j, fmt.Sprintf("git clone: %v", err), workspacePath)
				return
			}
		}
	}

	// Worker containers run as non-root (uid 1000); make the workspace writable.
	if err := ChmodWorkspace(workspacePath); err != nil {
		log.Printf("chmod workspace: %v", err)
	}

	if err := setupMise(workspacePath); err != nil {
		log.Printf("mise install: %v", err)
	}

	notify(bot, j.ChatID, fmt.Sprintf("🤖 [%s] %s — running AI agent on: %s", j.Provider, repoName, j.IssueTitle))

	var knowledgePath string
	var similarJobIDs []int64
	if pkgOllamaURL != "" {
		queryBody := j.IssueBody
		if len(queryBody) > 2000 {
			queryBody = queryBody[:2000]
		}
		// Use enhanced search with repository context for better matching
		similarJobs, err := knowledge.FindSimilarForRepo(ctx, pkgDB, pkgOllamaURL,
			j.IssueTitle+"\n"+queryBody, j.RepoURL, 5)
		if err != nil {
			log.Printf("knowledge lookup for job %d: %v", jobID, err)
		}
		if len(similarJobs) > 0 {
			knowledgePath, _ = knowledge.WriteKnowledgeFile(workspacePath, similarJobs)
			// Track similar job IDs for feedback
			for _, sj := range similarJobs {
				similarJobIDs = append(similarJobIDs, sj.ID)
			}
			// Track knowledge usage
			knowledge.TrackUsage(ctx, pkgDB, jobID, knowledgePath, similarJobIDs)
		}
	}

	// Run pre-implementation agent profiles (spec, plan, task, analyze).
	// Resumable: if profile_outputs already contains completed phases (e.g. after
	// an interruption or clarification), skip them and continue from the next one.
	profileOutputs := agent.ParseProfileOutputs(j.ProfileOutputs)
	priorOutputs := ""
	for _, p := range agent.ProfileOrder {
		out, ok := profileOutputs[string(p)]
		if !ok || out == "" {
			break
		}
		priorOutputs += fmt.Sprintf("\n## %s\n%s\n", p, out)
	}

	var promptPath string
	var rawOutput string
	agentText := profileOutputs[string(agent.ProfileImplement)]

	// Spec profile (may ask clarifying questions).
	if _, ok := profileOutputs[string(agent.ProfileSpec)]; !ok {
		notify(bot, j.ChatID, fmt.Sprintf("🤖 [%s] running: %s", j.Provider, agent.ProfileSpec.DisplayName()))
		specOut, err := runProfileWithHeartbeat(ctx, bot, j.ChatID, j.Provider, agent.ProfileSpec, j.Agent, j.AgentModel, workspacePath, agent.ProfilePrompt(agent.ProfileSpec, j.IssueTitle, j.IssueBody, workspacePath, priorOutputs), agentCfg)
		if err != nil {
			log.Printf("[runJob %d] profile %s failed: %v", jobID, agent.ProfileSpec, err)
			metrics.Global.Failed.Add(1)
			failJob(jobsDB, bot, j, fmt.Sprintf("Spec phase failed: %v", err), workspacePath)
			return
		}
		profileOutputs[string(agent.ProfileSpec)] = specOut
		if specOut != "" {
			priorOutputs += fmt.Sprintf("\n## %s\n%s\n", agent.ProfileSpec, specOut)
		}
		storage.SetJobProfileOutputs(jobsDB, jobID, profileOutputs.Marshal())

		// Post the spec as a comment on the issue before relaying to the next profile.
		if !agent.IsQuestion(specOut) && specOut != "" {
			comment := fmt.Sprintf("## Technical Specification\n\n%s", specOut)
			if err := provider.PostIssueComment(j.Provider, j.IssueURL, j.GitToken, comment); err != nil {
				log.Printf("[runJob %d] post spec comment: %v", jobID, err)
			} else {
				notify(bot, j.ChatID, fmt.Sprintf("📝 [%s] Spec posted as comment on the issue.", j.Provider))
			}
		}

		if agent.IsQuestion(specOut) {
			qc, _ := storage.GetQuestionCount(jobsDB, jobID)
			if qc < 3 {
				log.Printf("[runJob %d] spec profile asked a clarification question", jobID)
				if err := storage.SetJobAwaitingInput(jobsDB, jobID, specOut); err != nil {
					log.Printf("set awaiting_input: %v", err)
				}
				storage.SetConversationState(jobsDB, j.ChatID, "telegram", "await_agent_input",
					map[string]interface{}{"job_id": jobID})
				msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf(
					"🤔 [%s] Spec needs clarification:\n\n%s\n\nJust reply with your answer.",
					j.Provider, agent.ExtractQuestion(specOut),
				))
				sent, _ := bot.Send(msg)
				if sent.MessageID != 0 {
					storage.SetJobNotifMsgID(jobsDB, jobID, int64(sent.MessageID))
				}
				return
			}
			log.Printf("[runJob %d] spec asked a question but question limit reached; continuing", jobID)
		}
	}

	// Plan profile (run if not already completed).
	if _, ok := profileOutputs[string(agent.ProfilePlan)]; !ok {
		notify(bot, j.ChatID, fmt.Sprintf("🤖 [%s] running: %s", j.Provider, agent.ProfilePlan.DisplayName()))
		planOut, err := runProfileWithHeartbeat(ctx, bot, j.ChatID, j.Provider, agent.ProfilePlan, j.Agent, j.AgentModel, workspacePath, agent.ProfilePrompt(agent.ProfilePlan, j.IssueTitle, j.IssueBody, workspacePath, priorOutputs), agentCfg)
		if err != nil {
			log.Printf("[runJob %d] profile %s failed: %v", jobID, agent.ProfilePlan, err)
			metrics.Global.Failed.Add(1)
			failJob(jobsDB, bot, j, fmt.Sprintf("Plan phase failed: %v", err), workspacePath)
			return
		}
		profileOutputs[string(agent.ProfilePlan)] = planOut
		if planOut != "" {
			priorOutputs += fmt.Sprintf("\n## %s\n%s\n", agent.ProfilePlan, planOut)
			notify(bot, j.ChatID, fmt.Sprintf("📋 [%s] Plan posted as a comment on the issue: %s", j.Provider, j.IssueURL))
		}
		storage.SetJobProfileOutputs(jobsDB, jobID, profileOutputs.Marshal())
	}

	// Guard: do not proceed to tasks without a real plan. Re-run the plan
	// profile a couple of times if the output is missing/invalid.
	planOut := profileOutputs[string(agent.ProfilePlan)]
	retries := 0
	for !agent.IsValidPlan(planOut) && retries < 2 {
		retries++
		notify(bot, j.ChatID, fmt.Sprintf("⚠️ [%s] Plan output invalid (attempt %d); re-running plan phase...", j.Provider, retries))
		newOut, err := runProfileWithHeartbeat(ctx, bot, j.ChatID, j.Provider, agent.ProfilePlan, j.Agent, j.AgentModel, workspacePath, agent.ProfilePrompt(agent.ProfilePlan, j.IssueTitle, j.IssueBody, workspacePath, priorOutputs), agentCfg)
		if err != nil {
			log.Printf("[runJob %d] plan profile retry failed: %v", jobID, err)
			break
		}
		planOut = newOut
		profileOutputs[string(agent.ProfilePlan)] = planOut
		if planOut != "" {
			priorOutputs += fmt.Sprintf("\n## %s\n%s\n", agent.ProfilePlan, planOut)
		}
		storage.SetJobProfileOutputs(jobsDB, jobID, profileOutputs.Marshal())
	}
	if !agent.IsValidPlan(planOut) {
		metrics.Global.Failed.Add(1)
		failJob(jobsDB, bot, j, "plan profile produced no valid plan", workspacePath)
		return
	}

	// Plan review gate: ask user to confirm the plan before proceeding to tasks.
	// Auto-proceeds after 5 minutes if no response; feedback triggers a spec revision.
	if j.Status != "plan_confirmed" {
		planOut := profileOutputs[string(agent.ProfilePlan)]
		log.Printf("[runJob %d] plan review gate: planOut len=%d, status=%s", jobID, len(planOut), j.Status)
		if planOut != "" {
			comment := fmt.Sprintf("## Implementation Plan\n\n%s", planOut)
			if err := provider.PostIssueComment(j.Provider, j.IssueURL, j.GitToken, comment); err != nil {
				log.Printf("[runJob %d] post plan comment: %v", jobID, err)
			}
		} else {
			notify(bot, j.ChatID, fmt.Sprintf("⚠️ [%s] Plan phase produced no output.", j.Provider))
		}
		notify(bot, j.ChatID, fmt.Sprintf("📋 [%s] Plan ready. Confirm to proceed, or reply with feedback to revise the spec.\nAuto-continues in 5 minutes.", j.Provider))
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Confirm", "plan:confirm"),
				tgbotapi.NewInlineKeyboardButtonData("✏️ Request changes", "plan:revise"),
			),
		)
		msg := tgbotapi.NewMessage(j.ChatID, "Choose an action:")
		msg.ReplyMarkup = kb
		bot.Send(msg)
		storage.SetJobStatus(jobsDB, jobID, "awaiting_plan_approval")
		storage.SetConversationState(jobsDB, j.ChatID, "telegram", "await_plan_approval",
			map[string]interface{}{"job_id": jobID})

		// Auto-continue after 5 minutes if the user does nothing.
		go func() {
			time.Sleep(5 * time.Minute)
			jj, err := storage.GetJob(jobsDB, jobID)
			if err != nil || jj == nil || jj.Status != "awaiting_plan_approval" {
				return
			}
			log.Printf("[runJob %d] plan approval timed out; auto-continuing", jobID)
			storage.SetJobStatus(jobsDB, jobID, "plan_confirmed")
			storage.ResetConversation(jobsDB, jj.ChatID, "telegram")
			Run(context.Background(), jobID)
		}()

		// Stop here; continuation is triggered by the user (button/text) or the
		// auto-continue timer above.
		return
	}
	log.Printf("[runJob %d] plan review gate done; proceeding to task/analyze", jobID)
	notify(bot, j.ChatID, fmt.Sprintf("🤖 [%s] proceeding to task breakdown...", j.Provider))

	// Task and analyze (skip completed).
	for _, prof := range []agent.Profile{agent.ProfileTask, agent.ProfileAnalyze} {
		if _, ok := profileOutputs[string(prof)]; ok {
			continue
		}
		notify(bot, j.ChatID, fmt.Sprintf("🤖 [%s] running: %s", j.Provider, prof.DisplayName()))
		out, err := runProfileWithHeartbeat(ctx, bot, j.ChatID, j.Provider, prof, j.Agent, j.AgentModel, workspacePath, agent.ProfilePrompt(prof, j.IssueTitle, j.IssueBody, workspacePath, priorOutputs), agentCfg)
		if err != nil {
			log.Printf("[runJob %d] profile %s failed: %v", jobID, prof, err)
			out = fmt.Sprintf("phase %s failed: %v", prof, err)
		}
		profileOutputs[string(prof)] = out
		if out != "" {
			priorOutputs += fmt.Sprintf("\n## %s\n%s\n", prof, out)
		}
		storage.SetJobProfileOutputs(jobsDB, jobID, profileOutputs.Marshal())
	}

	// Implement phase (main code-changing run).
	if _, ok := profileOutputs[string(agent.ProfileImplement)]; !ok {
		basePrompt := agent.BuildPrompt(j.IssueTitle, j.IssueURL, j.IssueBody, filepath.Base(knowledgePath))
		implementPrompt := agent.ProfilePrompt(agent.ProfileImplement, j.IssueTitle, j.IssueBody, workspacePath, priorOutputs) + "\n" + basePrompt
		var err error
		promptPath, err = agent.WritePrompt(workspacePath, j.Agent, implementPrompt) // reuse promptPath
		_ = promptPath
		if err != nil {
			metrics.Global.Failed.Add(1)
			failJob(jobsDB, bot, j, fmt.Sprintf("write prompt: %v", err), workspacePath)
			return
		}

		heartbeatStop := make(chan struct{})
		var agentProc *os.Process
		var agentProcMu sync.Mutex
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
					agentProcMu.Lock()
					proc := agentProc
					agentProcMu.Unlock()
					if proc != nil {
						if err := proc.Signal(syscall.Signal(0)); err != nil {
							notify(bot, j.ChatID, fmt.Sprintf("⚠️ [%s] Agent process died after %d min — retrying", repoName, elapsed))
							return
						}
					}
					notify(bot, j.ChatID, fmt.Sprintf("⏳ [%s] Agent still working... (%d min elapsed)", repoName, elapsed))
				}
			}
		}()

		agentErr := withRetry(ctx, "agent run", 3, func() error {
			var e error
			rawOutput, e = agent.RunDetached(ctx, j.Agent, j.AgentModel, workspacePath, promptPath, agentCfg, func(p *os.Process) {
				agentProcMu.Lock()
				agentProc = p
				agentProcMu.Unlock()
			})
			return e
		})
		close(heartbeatStop)

		if agentErr != nil {
			metrics.Global.Failed.Add(1)
			failJob(jobsDB, bot, j, fmt.Sprintf("agent run: %v", agentErr), workspacePath)
			return
		}
		storage.SetJobAgentOutput(jobsDB, jobID, rawOutput)

		agentText = agent.ParseOutput(j.Agent, rawOutput)
		profileOutputs[string(agent.ProfileImplement)] = agentText
		priorOutputs += "\n## implement\n" + agentText
		storage.SetJobProfileOutputs(jobsDB, jobID, profileOutputs.Marshal())

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
	} else {
		if agentText != "" {
			// already completed earlier; still notify? skip to avoid spam
		}
	}

	// Converge phase.
	if _, ok := profileOutputs[string(agent.ProfileConverge)]; !ok {
		notify(bot, j.ChatID, fmt.Sprintf("🤖 [%s] running: %s", j.Provider, agent.ProfileConverge.DisplayName()))
		convergeOut, _ := runProfileWithHeartbeat(ctx, bot, j.ChatID, j.Provider, agent.ProfileConverge, j.Agent, j.AgentModel, workspacePath, agent.ProfilePrompt(agent.ProfileConverge, j.IssueTitle, j.IssueBody, workspacePath, priorOutputs), agentCfg)
		profileOutputs[string(agent.ProfileConverge)] = convergeOut
		priorOutputs += "\n## converge\n" + convergeOut
		storage.SetJobProfileOutputs(jobsDB, jobID, profileOutputs.Marshal())
	}

	// QA phase (after converge, before review).
	qaOutput := profileOutputs[string(agent.ProfileQA)]
	if _, ok := profileOutputs[string(agent.ProfileQA)]; !ok {
		notify(bot, j.ChatID, fmt.Sprintf("🤖 [%s] running: %s", j.Provider, agent.ProfileQA.DisplayName()))
		var qaErr error
		qaOutput, qaErr = runProfileWithHeartbeat(ctx, bot, j.ChatID, j.Provider, agent.ProfileQA, j.Agent, j.AgentModel, workspacePath, agent.ProfilePrompt(agent.ProfileQA, j.IssueTitle, j.IssueBody, workspacePath, priorOutputs), agentCfg)
		if qaErr != nil {
			metrics.Global.Failed.Add(1)
			failJob(jobsDB, bot, j, fmt.Sprintf("QA phase failed: %v", qaErr), workspacePath)
			return
		}
		profileOutputs[string(agent.ProfileQA)] = qaOutput
		if qaOutput != "" {
			priorOutputs += "\n## qa\n" + qaOutput
		}
		storage.SetJobProfileOutputs(jobsDB, jobID, profileOutputs.Marshal())
	}

	// Fix-and-reverify loop on blocking QA defects.
	qaIterations := 0
	if v, ok := profileOutputs["qa_fix_iterations"]; ok && v != "" {
		fmt.Sscanf(v, "%d", &qaIterations)
	}
	for agent.IsQAFailure(qaOutput) && qaIterations < agent.QAMaxFixIterations {
		qaIterations++
		profileOutputs["qa_fix_iterations"] = fmt.Sprintf("%d", qaIterations)
		storage.SetJobProfileOutputs(jobsDB, jobID, profileOutputs.Marshal())

		notify(bot, j.ChatID, fmt.Sprintf("🔁 [%s] QA found defects; re-running implement (iteration %d/%d)...", j.Provider, qaIterations, agent.QAMaxFixIterations))

		// Re-run implement phase.
		basePrompt := agent.BuildPrompt(j.IssueTitle, j.IssueURL, j.IssueBody, filepath.Base(knowledgePath))
		implementPrompt := agent.ProfilePrompt(agent.ProfileImplement, j.IssueTitle, j.IssueBody, workspacePath, priorOutputs) + "\n" + basePrompt
		promptPath2, err := agent.WritePrompt(workspacePath, j.Agent, implementPrompt)
		if err != nil {
			metrics.Global.Failed.Add(1)
			failJob(jobsDB, bot, j, fmt.Sprintf("write prompt: %v", err), workspacePath)
			return
		}

		heartbeatStop := make(chan struct{})
		var agentProc *os.Process
		var agentProcMu sync.Mutex
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
					agentProcMu.Lock()
					proc := agentProc
					agentProcMu.Unlock()
					if proc != nil {
						if err := proc.Signal(syscall.Signal(0)); err != nil {
							notify(bot, j.ChatID, fmt.Sprintf("⚠️ [%s] Agent process died after %d min — retrying", repoName, elapsed))
							return
						}
					}
					notify(bot, j.ChatID, fmt.Sprintf("⏳ [%s] Agent still working... (%d min elapsed)", repoName, elapsed))
				}
			}
		}()

		agentErr := withRetry(ctx, "agent run", 3, func() error {
			var e error
			rawOutput, e = agent.RunDetached(ctx, j.Agent, j.AgentModel, workspacePath, promptPath2, agentCfg, func(p *os.Process) {
				agentProcMu.Lock()
				agentProc = p
				agentProcMu.Unlock()
			})
			return e
		})
		close(heartbeatStop)
		os.Remove(promptPath2)

		if agentErr != nil {
			metrics.Global.Failed.Add(1)
			failJob(jobsDB, bot, j, fmt.Sprintf("agent run: %v", agentErr), workspacePath)
			return
		}
		storage.SetJobAgentOutput(jobsDB, jobID, rawOutput)

		agentText = agent.ParseOutput(j.Agent, rawOutput)
		profileOutputs[string(agent.ProfileImplement)] = agentText
		priorOutputs += "\n## implement\n" + agentText
		storage.SetJobProfileOutputs(jobsDB, jobID, profileOutputs.Marshal())

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

		// Re-run converge.
		notify(bot, j.ChatID, fmt.Sprintf("🤖 [%s] running: %s", j.Provider, agent.ProfileConverge.DisplayName()))
		convergeOut, convErr := runProfileWithHeartbeat(ctx, bot, j.ChatID, j.Provider, agent.ProfileConverge, j.Agent, j.AgentModel, workspacePath, agent.ProfilePrompt(agent.ProfileConverge, j.IssueTitle, j.IssueBody, workspacePath, priorOutputs), agentCfg)
		if convErr != nil {
			metrics.Global.Failed.Add(1)
			failJob(jobsDB, bot, j, fmt.Sprintf("Converge phase failed: %v", convErr), workspacePath)
			return
		}
		profileOutputs[string(agent.ProfileConverge)] = convergeOut
		priorOutputs += "\n## converge\n" + convergeOut
		storage.SetJobProfileOutputs(jobsDB, jobID, profileOutputs.Marshal())

		// Re-run QA.
		notify(bot, j.ChatID, fmt.Sprintf("🤖 [%s] running: %s", j.Provider, agent.ProfileQA.DisplayName()))
		var qaErr error
		qaOutput, qaErr = runProfileWithHeartbeat(ctx, bot, j.ChatID, j.Provider, agent.ProfileQA, j.Agent, j.AgentModel, workspacePath, agent.ProfilePrompt(agent.ProfileQA, j.IssueTitle, j.IssueBody, workspacePath, priorOutputs), agentCfg)
		if qaErr != nil {
			metrics.Global.Failed.Add(1)
			failJob(jobsDB, bot, j, fmt.Sprintf("QA phase failed: %v", qaErr), workspacePath)
			return
		}
		profileOutputs[string(agent.ProfileQA)] = qaOutput
		if qaOutput != "" {
			priorOutputs += "\n## qa\n" + qaOutput
		}
		storage.SetJobProfileOutputs(jobsDB, jobID, profileOutputs.Marshal())
	}

	if agent.IsQAFailure(qaOutput) {
		metrics.Global.Failed.Add(1)
		failJob(jobsDB, bot, j, "QA phase failed after fix iterations", workspacePath)
		return
	}

	// Reviewer phase.
	if _, ok := profileOutputs[string(agent.ProfileReviewer)]; !ok {
		notify(bot, j.ChatID, fmt.Sprintf("🤖 [%s] running: %s", j.Provider, agent.ProfileReviewer.DisplayName()))
		reviewOutput, _ := runProfileWithHeartbeat(ctx, bot, j.ChatID, j.Provider, agent.ProfileReviewer, j.Agent, j.AgentModel, workspacePath, agent.ProfilePrompt(agent.ProfileReviewer, j.IssueTitle, j.IssueBody, workspacePath, priorOutputs), agentCfg)
		profileOutputs[string(agent.ProfileReviewer)] = reviewOutput
		storage.SetJobProfileOutputs(jobsDB, jobID, profileOutputs.Marshal())
	}

	reviewOutput := profileOutputs[string(agent.ProfileReviewer)]

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
	if reviewOutput != "" {
		if err := provider.PostIssueComment(j.Provider, j.IssueURL, j.GitToken, "## Agent Review\n\n"+reviewOutput); err != nil {
			log.Printf("[runJob %d] post review comment: %v", jobID, err)
		}
	}
	if qaOutput != "" {
		if err := provider.PostIssueComment(j.Provider, j.IssueURL, j.GitToken, "## QA Report\n\n"+qaOutput); err != nil {
			log.Printf("[runJob %d] post qa comment: %v", jobID, err)
		}
	} else {
		notify(bot, j.ChatID, fmt.Sprintf("✅ [%s] QA passed — no defects found.", j.Provider))
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

	// Check for a queued follow-up message. If the user sent a follow-up while
	// the job was still running, process it now instead of waiting for a reply.
	conv, convErr := storage.GetConversation(jobsDB, j.ChatID, "telegram")
	if convErr == nil && conv.State == "await_followup" {
		queuedText, _ := conv.Data["queued_text"].(string)
		isQueued, _ := conv.Data["queued"].(bool)
		if isQueued && queuedText != "" {
			// Clear the conversation state before processing the follow-up.
			storage.ResetConversation(jobsDB, j.ChatID, "telegram")
			agentCfgCopy := *agentCfg
			go RunFollowUp(ctx, jobID, queuedText, jobsDB, bot, &agentCfgCopy)
			os.Remove(promptPath)
			if knowledgePath != "" {
				os.Remove(knowledgePath)
			}
			return
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
		if pkgOllamaURL == "" {
			return
		}
		updated, err := storage.GetJob(pkgDB, jobID)
		if err != nil {
			log.Printf("[knowledge] get job %d for embedding: %v", jobID, err)
			return
		}
		if err := knowledge.EmbedJob(context.Background(), pkgDB, pkgOllamaURL, *updated); err != nil {
			log.Printf("[knowledge] embed job %d failed: %v", jobID, err)
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

// ChmodWorkspace recursively opens workspace permissions so that non-root worker
// containers (uid 1000) can read and write files created by the root daemon.
func ChmodWorkspace(path string) error {
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
