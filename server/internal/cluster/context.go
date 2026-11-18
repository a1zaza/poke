package cluster

import (
	"context"
	"net/http"
)

type projectContextKey struct{}

func withProject(req *http.Request, source *ProjectSource) *http.Request {
	if source == nil {
		return req
	}
	return req.WithContext(context.WithValue(req.Context(), projectContextKey{}, source))
}
func OperationProject(ctx context.Context) *ProjectSource {
	p, _ := ctx.Value(projectContextKey{}).(*ProjectSource)
	return p
}
