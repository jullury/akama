# Context Optimization Reference

This document provides detailed technical reference for context optimization techniques and strategies.

## Compaction Strategies

### Summary-Based Compaction

Summary-based compaction replaces verbose content with concise summaries while preserving key information. The approach works by identifying sections that can be compressed, generating summaries that capture essential points, and replacing full content with summaries.

The effectiveness of compaction depends on what information is preserved. Critical decisions, user preferences, and current task state should never be compacted. Intermediate results and supporting evidence can be summarized more aggressively. Boilerplate, repeated information, and exploratory reasoning can often be removed entirely.

### Token Budget Allocation

Effective context budgeting requires understanding how different context components consume tokens and allocating budget strategically:

| Component | Typical Range | Notes |
|-----------|---------------|-------|
| System prompt | 500-2000 tokens | Stable across session |
| Tool definitions | 100-500 per tool | Grows with tool count |
| Retrieved documents | Variable | Often largest consumer |
| Message history | Variable | Grows with conversation |
| Tool outputs | Variable | Can dominate context |

### Compaction Thresholds

Trigger compaction at appropriate thresholds to maintain performance:

- Warning threshold at 70% of effective context limit
- Compaction trigger at 80% of effective context limit
- Aggressive compaction at 90% of effective context limit

The exact thresholds depend on model behavior and task characteristics. Some models show graceful degradation while others exhibit sharp performance cliffs.

## Observation Masking Patterns

### Selective Masking

Not all observations should be masked equally. Consider masking observations that have served their purpose and are no longer needed for active reasoning. Keep observations that are central to the current task. Keep observations from the most recent turn. Keep observations that may be referenced again.

### Masking Implementation

```python
def selective_mask(observations: List[Dict], current_task: Dict) -> List[Dict]:
    """
    Selectively mask observations based on relevance.
    
    Returns observations with mask field indicating masked content.
    """
    masked = []
    
    for obs in observations:
        relevance = calculate_relevance(obs, current_task)
        
        if relevance < 0.3 and obs["age"] > 3:
            # Low relevance and old - mask
            masked.append({
                **obs,
                "masked": True,
                "reference": store_for_reference(obs["content"]),
                "summary": summarize_content(obs["content"])
            })
        else:
            masked.append({
                **obs,
                "masked": False
            })
    
    return masked
