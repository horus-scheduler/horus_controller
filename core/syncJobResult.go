package core

// SyncJobResult ...
type SyncJobResult struct {
	SessionAddress []byte
	Receivers      []string

	JobType    SyncJobType
	MsgContent []byte
	SequenceID uint32
	timeIndex  float64
}

// NewSyncJobResult ...
func NewSyncJobResult(sessionAddress []byte, jobType SyncJobType, receivers []string, msgContent []byte, sequenceID uint32, timeIndex float64) *SyncJobResult {
	sr := &SyncJobResult{
		SessionAddress: sessionAddress,
		Receivers:      receivers,
		JobType:        jobType,
		MsgContent:     msgContent,
		SequenceID:     sequenceID,
		timeIndex:      timeIndex}
	return sr
}
