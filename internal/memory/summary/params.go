// Package summary 实现按话题组织的日摘要流水线与确定性校验。
package summary

const (
	sliceGapMinutes  = 30
	sliceMaxMsgs     = 200
	sliceMaxChars    = 8000
	jobMaxSlices     = 80
	sliceConcurrency = 4
	windowDays       = 3
	llmTemperature   = 0.2
)

const fillerWords = "|嗯|嗯嗯|哦|噢|啊|哈|呵|草|艹|好|好的|行|对|ok|1|"
