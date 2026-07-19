package main

import (
	"context"
	"encoding/json"

	"github.com/sourcegraph/jsonrpc2"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

const requestCancelledCode int64 = -32800

type activeRequest struct {
	context context.Context
	cancel  context.CancelFunc
	id      jsonrpc2.ID
}

type concurrentRequestHandler struct {
	handler jsonrpc2.Handler
}

func (handler concurrentRequestHandler) Handle(
	context context.Context,
	connection *jsonrpc2.Conn,
	request *jsonrpc2.Request,
) {
	if request.Notif {
		handler.handler.Handle(context, connection, request)
		return
	}

	go handler.handler.Handle(context, connection, request)
}

func (server *Server) registerRequest(
	parent context.Context,
	id jsonrpc2.ID,
	glspContext *glsp.Context,
) (context.Context, func()) {
	requestContext, cancel := context.WithCancel(parent)
	request := &activeRequest{
		context: requestContext,
		cancel:  cancel,
		id:      id,
	}

	server.requestsLock.Lock()
	previous := server.activeRequests[id]
	server.activeRequests[id] = request
	server.requestContexts[glspContext] = request
	server.requestsLock.Unlock()
	if previous != nil {
		previous.cancel()
	}

	finish := func() {
		server.requestsLock.Lock()
		if server.activeRequests[id] == request {
			delete(server.activeRequests, id)
		}
		delete(server.requestContexts, glspContext)
		server.requestsLock.Unlock()

		cancel()
	}
	return requestContext, finish
}

func (server *Server) cancelRequest(context *glsp.Context, params *protocol.CancelParams) error {
	id, ok := cancellationRequestID(params.ID)
	if !ok {
		return nil
	}

	server.cancelRequestID(id)
	return nil
}

func (server *Server) cancelRequestID(id jsonrpc2.ID) {
	server.requestsLock.Lock()
	request := server.activeRequests[id]
	server.requestsLock.Unlock()
	if request != nil {
		request.cancel()
	}
}

func (server *Server) requestContext(glspContext *glsp.Context) context.Context {
	server.requestsLock.Lock()
	request := server.requestContexts[glspContext]
	server.requestsLock.Unlock()
	if request == nil {
		return context.Background()
	}
	return request.context
}

func (server *Server) requestID(glspContext *glsp.Context) (jsonrpc2.ID, bool) {
	server.requestsLock.Lock()
	request := server.requestContexts[glspContext]
	server.requestsLock.Unlock()
	if request == nil {
		return jsonrpc2.ID{}, false
	}
	return request.id, true
}

func (server *Server) activeRequestCount() int {
	server.requestsLock.Lock()
	defer server.requestsLock.Unlock()
	return len(server.activeRequests)
}

func cancellationRequestID(id protocol.IntegerOrString) (jsonrpc2.ID, bool) {
	switch value := id.Value.(type) {
	case int32:
		return jsonrpc2.ID{Num: uint64(value)}, true
	case string:
		return jsonrpc2.ID{Str: value, IsString: true}, true
	default:
		return jsonrpc2.ID{}, false
	}
}

func cancellationRequestIDFromJSON(params json.RawMessage) (jsonrpc2.ID, bool) {
	var rawParams struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(params, &rawParams); err != nil || rawParams.ID == nil {
		return jsonrpc2.ID{}, false
	}

	var number protocol.Integer
	if err := json.Unmarshal(rawParams.ID, &number); err == nil {
		return jsonrpc2.ID{Num: uint64(number)}, true
	}

	var text string
	if err := json.Unmarshal(rawParams.ID, &text); err == nil {
		return jsonrpc2.ID{Str: text, IsString: true}, true
	}
	return jsonrpc2.ID{}, false
}

func requestCancellationError() *jsonrpc2.Error {
	return &jsonrpc2.Error{
		Code:    requestCancelledCode,
		Message: "request cancelled",
	}
}
