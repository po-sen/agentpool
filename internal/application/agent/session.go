package agent

import "time"

func (s *runSession) finalResult(summary string) RunResult {
	return RunResult{
		Summary:       summary,
		ToolCallCount: s.toolCallCount,
		ToolCalls:     copyToolCallRecords(s.toolCalls),
		AgentTurns:    copyTurnRecords(s.turnRecords),
	}
}

func (s *runSession) partialResult() RunResult {
	return RunResult{
		ToolCallCount: s.toolCallCount,
		ToolCalls:     copyToolCallRecords(s.toolCalls),
		AgentTurns:    copyTurnRecords(s.turnRecords),
	}
}

func (s *runSession) toolIsAvailable(name string) bool {
	if s.availableTools == nil {
		return false
	}

	_, ok := s.availableTools[name]

	return ok
}

func (s *runSession) recordToolCall(
	name string,
	arguments map[string]string,
	result string,
	isError bool,
	startedAt time.Time,
	endedAt time.Time,
) {
	s.toolCalls = append(s.toolCalls, ToolCallRecord{
		Name:      name,
		Arguments: copyArguments(arguments),
		Result:    result,
		IsError:   isError,
		StartedAt: startedAt,
		EndedAt:   endedAt,
	})
	s.notifyProgress()
}

func (s *runSession) nextTurnIndex() int {
	s.turnIndex++

	return s.turnIndex
}

func (s *runSession) recordTurn(record TurnRecord) {
	record.RequestMessages = copyTurnMessageRecords(record.RequestMessages)
	record.RawResponse = previewRawResponse(record.RawResponse)
	record.ResponsePreview = previewModelResponse(record.ResponsePreview)
	record.CorrectionMessage = previewCorrectionMessage(record.CorrectionMessage)
	if s.currentTurn != nil && s.currentTurn.Index == record.Index {
		s.currentTurn = nil
	}
	s.turnRecords = append(s.turnRecords, record)
	s.notifyProgress()
}

func (s *runSession) startTurnProgress(record TurnRecord) {
	if s.progressObserver == nil {
		return
	}

	record.RequestMessages = copyTurnMessageRecords(record.RequestMessages)
	record.RawResponse = previewRawResponse(record.RawResponse)
	record.ResponsePreview = previewModelResponse(record.ResponsePreview)
	record.CorrectionMessage = previewCorrectionMessage(record.CorrectionMessage)
	s.currentTurn = &record
	s.notifyProgress()
}

func (s *runSession) notifyProgress() {
	if s.progressObserver == nil || s.progressErr != nil {
		return
	}

	s.progressErr = s.progressObserver(s.ctx, RunProgress{
		ToolCallCount: s.toolCallCount,
		ToolCalls:     copyToolCallRecords(s.toolCalls),
		AgentTurns:    s.progressAgentTurns(),
	})
}

func (s *runSession) progressAgentTurns() []TurnRecord {
	records := copyTurnRecords(s.turnRecords)
	if s.currentTurn == nil || turnRecordIndexExists(records, s.currentTurn.Index) {
		return records
	}

	current := *s.currentTurn
	current.RequestMessages = copyTurnMessageRecords(current.RequestMessages)
	records = append(records, current)

	return records
}

func turnRecordIndexExists(records []TurnRecord, index int) bool {
	for _, record := range records {
		if record.Index == index {
			return true
		}
	}

	return false
}
