package bot

import "sync"

type Runner interface {
	Start()
	Stop()
}

type MultiRunner struct {
	runners []Runner
	once    sync.Once
}

func NewMultiRunner(runners ...Runner) *MultiRunner {
	filtered := make([]Runner, 0, len(runners))
	for _, r := range runners {
		if r != nil {
			filtered = append(filtered, r)
		}
	}
	return &MultiRunner{runners: filtered}
}

func (m *MultiRunner) Start() {
	m.once.Do(func() {
		var wg sync.WaitGroup
		for _, r := range m.runners {
			wg.Add(1)
			go func(r Runner) {
				defer wg.Done()
				r.Start()
			}(r)
		}
		wg.Wait()
	})
}

func (m *MultiRunner) Stop() {
	for _, r := range m.runners {
		r.Stop()
	}
}
