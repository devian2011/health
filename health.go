package health

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
)

var (
	ErrUnknownChecker           = errors.New("unknown checker")
	ErrCheckAlreadyExists       = errors.New("checker already exists")
	ErrOnLostFnAlreadyExists    = errors.New("on lost function already exists")
	ErrOnRestoreFnAlreadyExists = errors.New("on restore function already exists")
)

type IsConnectedFunction func() (any, error)

type Checker struct {
	code    string
	delay   time.Duration
	checkFn IsConnectedFunction
}

type CheckResult struct {
	Code     string
	LastCall time.Time
	Err      error
	Info     any
}

func (c *CheckResult) IsFirstCall() bool {
	return c.LastCall == time.Time{}
}

func (c *CheckResult) IsAvailable() bool {
	return c.Err == nil
}

func NewChecker(code string, delay time.Duration, checkFn IsConnectedFunction) *Checker {
	return &Checker{
		code:    code,
		delay:   delay,
		checkFn: checkFn,
	}
}

type Health struct {
	mtx      *sync.RWMutex
	ctx      context.Context
	checkers map[string]*Checker
	outCh    chan CheckResult
	results  map[string]CheckResult

	onLost    map[string]func(result CheckResult)
	onRestore map[string]func(result CheckResult)
}

func NewHealth(ctx context.Context, checkers map[string]*Checker) *Health {
	h := &Health{
		mtx:      &sync.RWMutex{},
		ctx:      ctx,
		checkers: checkers,
		outCh:    make(chan CheckResult),
		results:  make(map[string]CheckResult),

		onLost:    make(map[string]func(result CheckResult)),
		onRestore: make(map[string]func(result CheckResult)),
	}
	go h.collectResults()
	return h
}

func (h *Health) OnLostRegister(code string, fn func(result CheckResult)) error {
	if _, exists := h.onLost[code]; exists {
		return ErrOnLostFnAlreadyExists
	}
	h.onLost[code] = fn

	return nil
}

func (h *Health) OnRestoreRegister(code string, fn func(result CheckResult)) error {
	if _, exists := h.onRestore[code]; exists {
		return ErrOnLostFnAlreadyExists
	}
	h.onRestore[code] = fn

	return nil
}

func (h *Health) GetServices() []string {
	result := make([]string, len(h.results))
	for code := range h.results {
		result = append(result, code)
	}

	return result
}

func (h *Health) GetResults() map[string]CheckResult {
	return h.results
}

func (h *Health) AddChecker(checker *Checker) error {
	if _, exists := h.checkers[checker.code]; exists {
		return errors.Wrapf(ErrCheckAlreadyExists, "checker code: %s", checker.code)
	}

	h.checkers[checker.code] = checker
	h.results[checker.code] = CheckResult{
		Code:     checker.code,
		LastCall: time.Time{},
		Err:      nil,
	}
	return nil
}

func (h *Health) ManualCheck(code string) CheckResult {
	if checker, exists := h.checkers[code]; exists {
		info, err := checker.checkFn()
		h.mtx.Lock()
		h.results[code] = CheckResult{
			Code:     code,
			LastCall: time.Now(),
			Err:      err,
			Info:     info,
		}
		h.mtx.Unlock()

		return h.results[code]
	}

	return CheckResult{
		Code:     code,
		LastCall: time.Time{},
		Err:      ErrUnknownChecker,
		Info:     nil,
	}
}

func (h *Health) RunChecker(code string) {
	if _, exists := h.checkers[code]; !exists {
		return
	}
	go func(code string, ch *Checker, out chan CheckResult) {
		for {
			select {
			case <-h.ctx.Done():
				return
			case <-time.After(ch.delay):
				info, err := ch.checkFn()
				out <- CheckResult{
					Code:     code,
					LastCall: time.Now(),
					Err:      err,
					Info:     info,
				}
			}
		}
	}(code, h.checkers[code], h.outCh)
}

func (h *Health) notifyIfStateChanged(old CheckResult, new CheckResult) {
	if old.IsAvailable() && !new.IsAvailable() {
		if action, exists := h.onLost[new.Code]; exists {
			go action(new)
		}
	}

	if !old.IsAvailable() && new.IsAvailable() {
		if action, exists := h.onRestore[new.Code]; exists {
			go action(new)
		}
	}
}

func (h *Health) collectResults() {
	for r := range h.outCh {
		oldResult := h.results[r.Code]
		go h.notifyIfStateChanged(oldResult, r)

		h.mtx.Lock()
		h.results[r.Code] = r
		h.mtx.Unlock()
	}
}

func (h *Health) Run() {
	for code := range h.checkers {
		go h.RunChecker(code)
	}
}
