// pkg/context/llm_engine.go
package context

import (
	"strings"
)

type LLMContextEngine struct {
	memoryMgr *MemoryManager
}

type ContextChunk struct {
	Type          string // 'instruction', 'thought', 'inference', 'code', 'graph'
	Content       string
	Weight        float64 // важность (0-1)
	TokenEstimate int
}

func NewLLMContextEngine(memoryMgr *MemoryManager) *LLMContextEngine {
	return &LLMContextEngine{memoryMgr: memoryMgr}
}

// BuildContextForLLM — собирает оптимизированный контекст для LLM
func (e *LLMContextEngine) BuildContextForLLM(sessionID, versionID int, maxTokens int) string {
	chunks := e.gatherChunks(sessionID, versionID)
	e.sortByWeight(chunks)
	compressed := e.compress(chunks, maxTokens)
	return e.format(compressed)
}

func (e *LLMContextEngine) gatherChunks(sessionID, versionID int) []ContextChunk {
	var chunks []ContextChunk

	// 1. Инструкции (самые важные)
	instructions := e.memoryMgr.GetCurrentStack(sessionID, versionID)
	for _, inst := range instructions {
		chunks = append(chunks, ContextChunk{
			Type:          "instruction",
			Content:       inst.Content,
			Weight:        1.0,
			TokenEstimate: len(inst.Content) / 4,
		})
	}

	// 2. Выводы с высоким confidence
	inferences := e.memoryMgr.GetActiveInferences(sessionID, versionID, 0.7)
	for _, inf := range inferences {
		chunks = append(chunks, ContextChunk{
			Type:          "inference",
			Content:       inf.Conclusion,
			Weight:        inf.Confidence,
			TokenEstimate: len(inf.Conclusion) / 4,
		})
	}

	// 3. Недавние мысли
	thoughts := e.memoryMgr.GetRecentThoughts(sessionID, versionID, 10)
	for _, thought := range thoughts {
		weight := 0.5
		if thought.Type == "decision" {
			weight = 0.8
		}
		chunks = append(chunks, ContextChunk{
			Type:          "thought",
			Content:       thought.Content,
			Weight:        weight,
			TokenEstimate: len(thought.Content) / 4,
		})
	}

	// 4. Буфер (только ключи)
	bufferKeys := e.memoryMgr.GetBufferKeys(sessionID, versionID)
	if len(bufferKeys) > 0 {
		chunks = append(chunks, ContextChunk{
			Type:          "buffer",
			Content:       "Доступные данные: " + strings.Join(bufferKeys, ", "),
			Weight:        0.3,
			TokenEstimate: len(bufferKeys) * 2,
		})
	}

	return chunks
}

func (e *LLMContextEngine) sortByWeight(chunks []ContextChunk) {
	for i := 0; i < len(chunks)-1; i++ {
		for j := i + 1; j < len(chunks); j++ {
			if chunks[i].Weight < chunks[j].Weight {
				chunks[i], chunks[j] = chunks[j], chunks[i]
			}
		}
	}
}

func (e *LLMContextEngine) compress(chunks []ContextChunk, maxTokens int) []ContextChunk {
	var result []ContextChunk
	totalTokens := 0

	for _, chunk := range chunks {
		if totalTokens+chunk.TokenEstimate <= maxTokens {
			result = append(result, chunk)
			totalTokens += chunk.TokenEstimate
		} else if chunk.Type == "instruction" {
			truncated := e.truncateToFit(chunk.Content, maxTokens-totalTokens)
			result = append(result, ContextChunk{
				Type:          chunk.Type,
				Content:       truncated,
				Weight:        chunk.Weight,
				TokenEstimate: maxTokens - totalTokens,
			})
			break
		}
	}

	return result
}

func (e *LLMContextEngine) truncateToFit(content string, maxTokens int) string {
	maxChars := maxTokens * 4
	if len(content) <= maxChars {
		return content
	}
	return content[:maxChars-3] + "..."
}

func (e *LLMContextEngine) format(chunks []ContextChunk) string {
	var sb strings.Builder

	for _, chunk := range chunks {
		switch chunk.Type {
		case "instruction":
			sb.WriteString("=== 🎯 ИНСТРУКЦИЯ ===\n")
			sb.WriteString(chunk.Content)
			sb.WriteString("\n\n")
		case "inference":
			sb.WriteString("=== ✅ ВЫВОД ===\n")
			sb.WriteString(chunk.Content)
			sb.WriteString("\n\n")
		case "thought":
			sb.WriteString("=== 💭 МЫСЛЬ ===\n")
			sb.WriteString(chunk.Content)
			sb.WriteString("\n\n")
		case "buffer":
			sb.WriteString("=== 📦 ДОСТУПНЫЕ ДАННЫЕ ===\n")
			sb.WriteString(chunk.Content)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}
