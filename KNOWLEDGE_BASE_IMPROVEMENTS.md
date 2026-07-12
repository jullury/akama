# Knowledge Base Improvements for Better Issue Resolution

## Overview

This document outlines the comprehensive improvements made to ensure the knowledge base in the vector database is effectively used on every issue created, enabling the system to better understand **why** issues happen and **how** to implement fixes based on prior art.

## Key Improvements

### 1. Enhanced Embedding Text (`internal/knowledge/embedder.go`)

**Before**: Embedding only included issue title, body, and plan.
**After**: Embedding now includes:
- Repository URL and provider (for better same-repo matching)
- Agent output containing reasoning and steps (1500 chars)
- Error messages for failed jobs
- All existing fields (title, body, plan, PR URL, branch)

**Impact**: Better similarity matching because embeddings now capture the full context of how issues were resolved, not just what the issue was about.

### 2. Repository-Weighted Similarity Search (`internal/knowledge/retriever.go`)

**Before**: `FindSimilar()` searched across all repos equally.
**After**: New `FindSimilarForRepo()` function:
- Weights same-repo matches 0.5 units higher in cosine distance
- Returns up to 5 results (increased from 3)
- Maintains backward compatibility with existing `FindSimilar()` function

**Impact**: Same-repo matches are prioritized, providing more relevant prior art for the specific codebase being worked on.

### 3. Structured Knowledge File with Root Cause Analysis (`internal/knowledge/writer.go`)

**Before**: Knowledge file was a simple dump of issue body, plan, and agent output.
**After**: Enhanced knowledge file includes:
- **Issue Classification**: Automatically categorizes issues (Bug Fix, Feature, Security, Performance, etc.)
- **Root Cause Analysis**: Extracts root cause patterns from agent output
- **Resolution Pattern**: Identifies the approach used to fix similar issues
- **Files Modified**: Detects which files were changed (when possible)
- **Key Takeaways**: Structured section for learning points
- **Analysis Instructions**: Explicit guidance on how to use the knowledge

**Impact**: Agents receive structured, actionable insights rather than raw data, enabling better decision-making.

### 4. Enhanced Agent Prompt (`internal/agent/runner.go`)

**Before**: Prompt simply said "read it before implementing."
**After**: Prompt explicitly instructs agents to:
1. **Identify Root Cause**: Understand WHY similar issues occurred
2. **Recognize Patterns**: Look for recurring patterns across similar issues
3. **Apply Proven Solutions**: Use approaches that worked for similar problems
4. **Prevent Recurrence**: Consider prevention strategies

**Impact**: Agents are now explicitly guided to analyze and apply knowledge, not just reference it.

### 5. Knowledge Usage Tracking (`internal/knowledge/tracker.go`)

**New Feature**: Comprehensive tracking of knowledge base effectiveness:
- Tracks which jobs used knowledge base
- Records similar jobs found and their IDs
- Monitors whether agents actually referenced the knowledge
- Provides statistics on knowledge base usage and effectiveness

**Database Schema Addition**:
```sql
CREATE TABLE knowledge_usage (
    id                 BIGSERIAL PRIMARY KEY,
    job_id             BIGINT UNIQUE REFERENCES jobs(id) ON DELETE CASCADE,
    knowledge_file_path TEXT,
    similar_jobs_found  INT DEFAULT 0,
    similar_job_ids     TEXT,
    agent_referenced    BOOLEAN DEFAULT FALSE,
    created_at         TIMESTAMPTZ DEFAULT NOW()
);
```

**Impact**: Enables monitoring and optimization of knowledge base effectiveness.

## How It Works End-to-End

### 1. Issue Creation
```
User sends issue URL
  → System fetches issue details
  → Creates job record
  → Starts job execution
```

### 2. Knowledge Retrieval
```
Job execution starts
  → Embeds issue title + body + context
  → Searches vector DB for similar completed jobs
  → Weights same-repo matches higher
  → Returns top 5 most relevant similar jobs
  → Writes structured knowledge file
  → Tracks usage in knowledge_usage table
```

### 3. Agent Execution
```
Agent receives prompt with:
  → Issue details
  → Reference to knowledge file
  → Explicit instructions to analyze root causes
  → Guidance to apply proven patterns
  
Agent processes:
  → Reads knowledge file
  → Analyzes similar issues
  → Identifies patterns and root causes
  → Implements fix based on prior art
  → Documents reasoning
```

### 4. Knowledge Embedding
```
After job completes successfully:
  → Embeds job with enhanced context
  → Stores embedding for future similarity searches
  → Updates knowledge base with new resolution pattern
```

## Benefits

### Better Issue Understanding
- **Root Cause Analysis**: System now captures WHY issues happen, not just what they are
- **Pattern Recognition**: Recurring issues are identified across the codebase
- **Context-Aware Matching**: Repository-specific patterns are prioritized

### Improved Implementation Quality
- **Proven Solutions**: Agents apply approaches that worked for similar issues
- **Prevention Strategies**: System learns to prevent recurring problems
- **Consistent Patterns**: Codebase conventions are maintained through prior art

### Measurable Effectiveness
- **Usage Tracking**: Know which jobs benefited from knowledge base
- **Effectiveness Metrics**: Measure impact on success rates
- **Continuous Improvement**: Data-driven optimization of knowledge base

## Configuration

### Required Components
- **Ollama**: Running with `nomic-embed-text` model
- **PostgreSQL**: With pgvector extension
- **Docker**: Infrastructure containers (optional but recommended)

### Configuration Parameters
```yaml
ollama_url: "http://localhost:11434"  # Or Docker internal URL
postgres_url: "postgresql://..."      # Connection string
```

### Graceful Degradation
- If Ollama is unavailable, system continues without knowledge base
- If no similar jobs found, system proceeds with standard implementation
- All knowledge operations are non-blocking

## Monitoring and Optimization

### Usage Statistics
```go
stats, err := knowledge.GetUsageStats(ctx, db)
// Returns:
// - total_jobs_with_knowledge
// - agent_referenced_knowledge
// - avg_similar_jobs_found
// - jobs_with_knowledge_succeeded
```

### Effective Similar Jobs
```go
effectiveJobs, err := knowledge.GetMostEffectiveSimilarJobs(ctx, db, 10)
// Returns IDs of jobs that led to successful fixes
```

## Future Enhancements

### Potential Improvements
1. **Semantic Similarity Weighting**: Use LLM to assess relevance of similar issues
2. **Repository-Specific Models**: Train embedding models on specific codebases
3. **Feedback Integration**: Allow agents to rate knowledge usefulness
4. **Automated Pattern Extraction**: Use LLM to extract patterns from successful fixes
5. **Cross-Repository Learning**: Identify patterns across related repositories

### Metrics to Track
- Knowledge base usage rate
- Agent reference rate
- Success rate with/without knowledge
- Time to resolution with prior art
- Pattern reuse frequency

## Summary

These improvements transform the knowledge base from a simple reference tool into an intelligent system that:

1. **Understands Context**: Embeddings capture full issue resolution context
2. **Prioritizes Relevance**: Same-repo matches are weighted higher
3. **Provides Structure**: Knowledge files contain actionable insights
4. **Guides Analysis**: Agents are explicitly instructed to analyze root causes
5. **Measures Effectiveness**: Usage tracking enables continuous improvement

The system now better understands **why** issues happen and **how** to implement fixes based on proven patterns from similar resolved issues.
