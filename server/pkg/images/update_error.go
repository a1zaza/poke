package images

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/docker/docker/errdefs"
)

func boundedErrorDetails(err error) string {
	text := []rune(err.Error())
	if len(text) > 4096 {
		return string(text[:4096]) + "…"
	}
	return string(text)
}

func registryCheckFailure(domain string, configured bool, err error) *UpdateResult {
	result := &UpdateResult{LastChecked: time.Now(), Registry: domain, AuthConfigured: configured, Details: boundedErrorDetails(err)}
	text := strings.ToLower(err.Error())
	authFailed := errdefs.IsUnauthorized(err) || errdefs.IsForbidden(err) || strings.Contains(text, "unauthorized") || strings.Contains(text, "authentication required") || strings.Contains(text, "insufficient_scope") || strings.Contains(text, "forbidden") || strings.Contains(text, "denied")
	html := strings.Contains(text, "<html") || strings.Contains(text, "<!doctype html")
	missingManifest := strings.Contains(text, "manifest unknown") || strings.Contains(text, "manifest_unknown") || strings.Contains(text, "name unknown") || strings.Contains(text, "name_unknown")
	notFound := errdefs.IsNotFound(err) || missingManifest || strings.Contains(text, "404 not found")
	var networkError net.Error
	connectionFailed := errors.As(err, &networkError) || errors.Is(err, context.DeadlineExceeded) || strings.Contains(text, "x509:") || strings.Contains(text, "tls:") || strings.Contains(text, "dial tcp") || strings.Contains(text, "timeout") || strings.Contains(text, "no such host")
	switch {
	case connectionFailed:
		result.ErrorCode = "REGISTRY_CONNECTION_FAILED"
		result.Error = fmt.Sprintf("无法连接镜像仓库 %s，请检查 Agent 主机的网络、DNS 和 TLS 证书。", domain)
	case !configured && (authFailed || notFound):
		result.ErrorCode = "REGISTRY_AUTH_MISSING"
		if authFailed {
			result.Error = fmt.Sprintf("仓库 %s 拒绝匿名访问，当前节点未配置认证或未获仓库授权。请在系统设置中保存仓库凭据，并授权给当前 Agent。", domain)
		} else {
			result.Error = fmt.Sprintf("仓库 %s 返回 404，当前节点未配置认证或未获仓库授权。请先配置或授权后重试；该响应也可能由镜像不存在或代理路径错误引起。", domain)
		}
	case authFailed:
		result.ErrorCode = "REGISTRY_AUTH_FAILED"
		result.Error = fmt.Sprintf("仓库 %s 拒绝已提供的认证信息，请检查用户名、密码或 Token，以及镜像读取权限。", domain)
	case html || (notFound && !missingManifest):
		result.ErrorCode = "REGISTRY_ENDPOINT_ERROR"
		result.Error = fmt.Sprintf("仓库 %s 返回了非预期的错误响应，请检查仓库入口、反向代理路径和访问权限；无法据此确认镜像是否存在。", domain)
	case notFound:
		result.ErrorCode = "REGISTRY_IMAGE_NOT_FOUND"
		result.Error = fmt.Sprintf("仓库 %s 未找到该镜像或标签，或当前账号没有访问权限，请核对镜像地址和权限。", domain)
	default:
		result.ErrorCode = "REGISTRY_CHECK_FAILED"
		result.Error = fmt.Sprintf("访问仓库 %s 的镜像信息失败，请查看原始错误并检查仓库服务。", domain)
	}
	return result
}
