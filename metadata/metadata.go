package metadata

import (
	log "github.com/Sirupsen/logrus"
	"sync"
)

type Service struct {
	data map[string]interface{}
	stop chan<- struct{}
	done <-chan struct{}
	lock sync.RWMutex
}

func NewService() (*Service, error) {
	s := &Service{
		data: map[string]interface{}{},
	}
	return s, nil
}

func (s *Service) Get(key string) interface{} {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.data[key]
}

func (s *Service) Set(key string, value interface{}) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.data[key] = value
}

func (s *Service) Copy(m map[string]interface{}) {
	s.lock.Lock()
	defer s.lock.Unlock()
	for k, v := range m {
		s.data[k] = v
	}
}

func (s *Service) Run(update <-chan map[string]interface{}) {

	stop := make(chan struct{})
	s.stop = stop

	done := make(chan struct{})
	s.done = done

	defer close(done)

	for {
		select {

		case <-stop:
			log.Infoln("Stopping.")
			return

		case update := <-update:
			s.Copy(update)

		}
	}
}

func (s *Service) Stop() {
	if s.stop != nil {
		close(s.stop)
	}

}

func (s *Service) Wait() {
	if s.done != nil {
		<-s.done
	}
}
