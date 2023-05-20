package health

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestHealth_ManualCheck(t *testing.T) {
	checkErr := errors.New("check err")
	health := NewHealth(context.Background(), map[string]*Checker{
		"one": {
			code:  "one",
			delay: time.Nanosecond,
			checkFn: func() (any, error) {
				return "check", nil
			},
		},
		"two": {
			code:  "two",
			delay: time.Nanosecond,
			checkFn: func() (any, error) {
				return nil, checkErr
			},
		},
	})
	result := health.ManualCheck("one")
	assert.Equal(t, "check", result.Info.(string))

	result = health.ManualCheck("two")
	assert.Equal(t, checkErr, result.Err)
}

func TestHealth_collectResults(t *testing.T) {
	health := NewHealth(context.Background(), map[string]*Checker{})

	onLostResult := false
	onRestoreResult := false

	health.results = map[string]CheckResult{
		"one": {
			Code:     "one",
			LastCall: time.Now(),
			Err:      nil,
			Info:     nil,
		},
	}

	health.onLost["one"] = func(result CheckResult) {
		onLostResult = true
	}

	health.onRestore["one"] = func(result CheckResult) {
		onRestoreResult = true
	}

	health.outCh <- CheckResult{
		Code:     "one",
		LastCall: time.Time{},
		Err:      errors.New("err"),
		Info:     nil,
	}

	health.outCh <- CheckResult{
		Code:     "one",
		LastCall: time.Time{},
		Err:      nil,
		Info:     nil,
	}
	// Wait goroutines done
	time.Sleep(1 * time.Second)

	assert.True(t, onLostResult)
	assert.True(t, onRestoreResult)
}
