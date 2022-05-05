package vlog

import (
	"github.com/stretchr/testify/suite"
	"testing"
	"time"
)

var (
	asyncLogChanTest = make(chan *AsyncFilterLogItem, 100)
)

func TestAPITestSuite(t *testing.T) {
	suite.Run(t, new(APITestSuite))
}

type APITestSuite struct {
	suite.Suite
}

func (s *APITestSuite) SetupTest() {
	go func() {
		for {
			select {
			case <-asyncLogChanTest:
			}
		}
	}()
}

func (s *APITestSuite) TestFilter() {
	f1 := &testAsyncFilter{
		channel:        asyncLogChanTest,
		requiredStack:  false,
		requiredCaller: false,
		callerSkip:     0,
	}
	AddAsyncFilter("test1", f1)
	doAsyncFilters(InfoLevel, "", "", "3123")
	doAsyncFilters(InfoLevel, "", "", "3123")
	doAsyncFilters(InfoLevel, "", "", "3123")
	doAsyncFilters(InfoLevel, "", "", "3123")
	time.Sleep(1)

	f2 := &testAsyncFilter{
		channel:        asyncLogChanTest,
		requiredStack:  true,
		requiredCaller: false,
		callerSkip:     0,
	}
	AddAsyncFilter("test2", f2)
	doAsyncFilters(InfoLevel, "", "", "3123")
	time.Sleep(1)

	f3 := &testAsyncFilter{
		channel:        asyncLogChanTest,
		requiredStack:  true,
		requiredCaller: true,
		callerSkip:     1,
	}
	AddAsyncFilter("test3", f3)
	doAsyncFilters(InfoLevel, "", "", "3123")
	time.Sleep(1)
}

type testAsyncFilter struct {
	channel        chan *AsyncFilterLogItem
	requiredStack  bool
	requiredCaller bool
	callerSkip     int
}

func (f *testAsyncFilter) StackRequired(level LogLevel, name string) bool {
	return f.requiredStack
}

func (f *testAsyncFilter) CallerRequired(level LogLevel, name string) (bool, int) {
	return f.requiredCaller, f.callerSkip
}

func (f *testAsyncFilter) GetChannel() chan *AsyncFilterLogItem {
	return f.channel
}
