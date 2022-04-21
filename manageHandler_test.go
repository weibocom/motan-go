package motan

import (
	assert2 "github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSleep(t *testing.T) {
	start := time.Now()
	sleep(&httptest.ResponseRecorder{}, time.Second*3)
	end := time.Now()
	assert2.True(t, end.Sub(start).Seconds() >= 3)
}
