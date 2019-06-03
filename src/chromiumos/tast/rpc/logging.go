package rpc

import (
	"container/list"
	"context"
	"sync"
)

type LoggingServerImpl struct {
	mu  sync.Mutex
	sss map[*logSubscriber]struct{}
}

func NewLoggingServerImpl() *LoggingServerImpl {
	sv := &LoggingServerImpl{
		sss: make(map[*logSubscriber]struct{}),
	}
	return sv
}

func (sv *LoggingServerImpl) Log(msg string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	for ss := range sv.sss {
		ss.Log(msg)
	}
}

func (sv *LoggingServerImpl) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, "logger", sv.Log)
}

func (sv *LoggingServerImpl) ReadLogs(req *ReadLogsRequest, srv Logging_ReadLogsServer) error {
	ctx := srv.Context()

	ss := newLogSubscriber()
	defer ss.Close()

	sv.subscribe(ss)
	defer sv.unsubscribe(ss)

	for {
		select {
		case msg := <-ss.C:
			if err := srv.Send(&ReadLogsReply{Msg: msg}); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (sv *LoggingServerImpl) subscribe(ss *logSubscriber) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	sv.sss[ss] = struct{}{}
}

func (sv *LoggingServerImpl) unsubscribe(ss *logSubscriber) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	delete(sv.sss, ss)
}

var _ LoggingServer = (*LoggingServerImpl)(nil)

type logSubscriber struct {
	C  <-chan string
	in chan<- string
}

func newLogSubscriber() *logSubscriber {
	out := make(chan string)
	in := make(chan string)
	ss := &logSubscriber{C: out, in: in}
	go relayLoop(out, in)
	return ss
}

func (ss *logSubscriber) Close() {
	close(ss.in)
}

func (ss *logSubscriber) Log(msg string) {
	ss.in <- msg
}

func relayLoop(out chan<- string, in <-chan string) {
	var buf list.List
	for {
		if buf.Len() == 0 {
			msg, ok := <-in
			if !ok {
				return
			}
			buf.PushBack(msg)
			continue
		}

		select {
		case msg, ok := <-in:
			if !ok {
				return
			}
			buf.PushBack(msg)
		case out <- buf.Front().Value.(string):
			buf.Remove(buf.Front())
		}
	}
}
