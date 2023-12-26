package vlog

import (
	"bytes"
	"flag"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

var logObject *AccessLogEntity
var expectString string

func init() {
	expectString = "FilterName|Role|100|Service|Method|Desc|RemoteAddress|100|100|100|100|false|Exception"
	logObject = &AccessLogEntity{
		FilterName:    "FilterName",
		Role:          "Role",
		RequestID:     100,
		Service:       "Service",
		Method:        "Method",
		Desc:          "Desc",
		RemoteAddress: "RemoteAddress",
		ReqSize:       100,
		ResSize:       100,
		BizTime:       100,
		TotalTime:     100,
		Success:       false,
		Exception:     "Exception"}
}

// BenchmarkLogSprintf: 736 ns/op
func BenchmarkLogSprintf(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fmt.Sprintf("%s|%s|%d|%s|%s|%s|%s|%d|%d|%d|%d|%t|%s",
			logObject.FilterName,
			logObject.Role,
			logObject.RequestID,
			logObject.Service,
			logObject.Method,
			logObject.Desc,
			logObject.RemoteAddress,
			logObject.ReqSize,
			logObject.ResSize,
			logObject.BizTime,
			logObject.TotalTime,
			logObject.Success,
			logObject.Exception)
	}
}

// BenchmarkLogBufferWritePlus: 438 ns/op
func BenchmarkLogBufferWritePlus(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buffer bytes.Buffer
		buffer.WriteString(
			logObject.FilterName + "|" +
				logObject.Role + "|" +
				strconv.FormatUint(logObject.RequestID, 10) + "|" +
				logObject.Service + "|" +
				logObject.Method + "|" +
				logObject.Desc + "|" +
				logObject.RemoteAddress + "|" +
				strconv.Itoa(logObject.ReqSize) + "|" +
				strconv.Itoa(logObject.ResSize) + "|" +
				strconv.FormatInt(logObject.BizTime, 10) + "|" +
				strconv.FormatInt(logObject.TotalTime, 10) + "|" +
				strconv.FormatBool(logObject.Success) + "|" +
				logObject.Exception)
	}
}

// BenchmarkLogBufferWrite: 406 ns/op
func BenchmarkLogBufferWrite(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buffer bytes.Buffer
		buffer.WriteString(logObject.FilterName)
		buffer.WriteString("|")
		buffer.WriteString(logObject.Role)
		buffer.WriteString("|")
		buffer.WriteString(strconv.FormatUint(logObject.RequestID, 10))
		buffer.WriteString("|")
		buffer.WriteString(logObject.Service)
		buffer.WriteString("|")
		buffer.WriteString(logObject.Method)
		buffer.WriteString("|")
		buffer.WriteString(logObject.Desc)
		buffer.WriteString("|")
		buffer.WriteString(logObject.RemoteAddress)
		buffer.WriteString("|")
		buffer.WriteString(strconv.Itoa(logObject.ReqSize))
		buffer.WriteString("|")
		buffer.WriteString(strconv.Itoa(logObject.ResSize))
		buffer.WriteString("|")
		buffer.WriteString(strconv.FormatInt(logObject.BizTime, 10))
		buffer.WriteString("|")
		buffer.WriteString(strconv.FormatInt(logObject.TotalTime, 10))
		buffer.WriteString("|")
		buffer.WriteString(strconv.FormatBool(logObject.Success))
		buffer.WriteString("|")
		buffer.WriteString(logObject.Exception)
	}
}

func TestAccessLog(t *testing.T) {
	var buffer bytes.Buffer
	buffer.WriteString(logObject.FilterName)
	buffer.WriteString("|")
	buffer.WriteString(logObject.Role)
	buffer.WriteString("|")
	buffer.WriteString(strconv.FormatUint(logObject.RequestID, 10))
	buffer.WriteString("|")
	buffer.WriteString(logObject.Service)
	buffer.WriteString("|")
	buffer.WriteString(logObject.Method)
	buffer.WriteString("|")
	buffer.WriteString(logObject.Desc)
	buffer.WriteString("|")
	buffer.WriteString(logObject.RemoteAddress)
	buffer.WriteString("|")
	buffer.WriteString(strconv.Itoa(logObject.ReqSize))
	buffer.WriteString("|")
	buffer.WriteString(strconv.Itoa(logObject.ResSize))
	buffer.WriteString("|")
	buffer.WriteString(strconv.FormatInt(logObject.BizTime, 10))
	buffer.WriteString("|")
	buffer.WriteString(strconv.FormatInt(logObject.TotalTime, 10))
	buffer.WriteString("|")
	buffer.WriteString(strconv.FormatBool(logObject.Success))
	buffer.WriteString("|")
	buffer.WriteString(logObject.Exception)
	assert.Equal(t, buffer.String(), expectString)
}

func TestChangeLogBufferSize(t *testing.T) {
	testLogger := newDefaultLog()
	testLogger.SetAsync(true)
	assert.Equal(t, cap(testLogger.(*defaultLogger).outputChan), 5000)
	_ = flag.Set("log_buffer_size", "1")
	testLogger = newDefaultLog()
	testLogger.SetAsync(true)
	assert.Equal(t, cap(testLogger.(*defaultLogger).outputChan), 1)
}
