package main

type Sentinel struct {
	Client          Client
	executorsByName map[string]Executor
	executorsByKey  map[string][]Executor
}

// Add an `executor` for the provided `keys`.
func (s *Sentinel) Add(keys []string, executor Executor) {
	if s.executorsByName == nil {
		s.executorsByName = make(map[string]Executor)
	}
	if s.executorsByKey == nil {
		s.executorsByKey = make(map[string][]Executor)
	}

	name := executor.Name()
	s.executorsByName[name] = executor
	for _, key := range keys {
		logger.Debugf("changes to %s will execute %s", key, name)
		executorArray, ok := s.executorsByKey[key]
		if !ok {
			executorArray = make([]Executor, 0, 1)
		}
		s.executorsByKey[key] = append(executorArray, executor)
	}
}

// Look up a executors by key and execute them.
func (s *Sentinel) executeKey(key string) {
	if executors, ok := s.executorsByKey[key]; ok {
		for _, executor := range executors {
			executor.Execute(s.Client)
		}
	}
}

// Get the prefixes we're configured to watch.
func (s *Sentinel) getPrefixes() []string {
	prefixes := make([]string, 0, len(s.executorsByKey))
	for prefix := range s.executorsByKey {
		prefixes = append(prefixes, prefix)
	}
	return prefixes
}

// Execute the named executors. If `names` is empty all executors will be run.
// A failed executor will not cause subsequent executors to be skipped.
// Failures are logged. Return true if all executors succeeded.
func (s *Sentinel) Execute(names []string) bool {
	success := true
	if len(names) == 0 {
		for _, executor := range s.executorsByName {
			if err := executor.Execute(s.Client); err != nil {
				logger.Errorf("executor %s failed: %s", executor.Name(), err)
				success = false
			}
		}
	} else {
		for _, name := range names {
			if _, ok := s.executorsByName[name]; !ok {
				logger.Errorf("executor %s not found", name)
				return false
			}
		}
		for _, name := range names {
			executor := s.executorsByName[name]
			if err := executor.Execute(s.Client); err != nil {
				logger.Errorf("executor %s failed: %s", executor.Name(), err)
				success = false
			}
		}
	}
	return success
}

func (s *Sentinel) Run(stop chan bool) {
	changes := make(chan string, 10)
	watchStop := make(chan bool)
	watchJoin := make(chan struct{})
	go func() {
		s.Client.Watch(s.getPrefixes(), changes, watchStop)
		close(watchJoin)
	}()

Loop:
	for {
		select {
		case <-stop:
			watchStop <- true
			<-watchJoin
			break Loop
		case prefix := <-changes:
			logger.Debugf("prefix '%s' changed", prefix)
			s.executeKey(prefix)
		}
	}
}
