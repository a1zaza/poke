package compose

import (
	"errors"
	"gopkg.in/yaml.v3"
	"regexp"
	"strings"
)

var Name = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var interpolation = regexp.MustCompile(`\$[A-Za-z_{]`)

// Remote execution accepts self-contained sources only. Never resolve includes,
// host .env files or build contexts supplied indirectly through node filesystem.
func Validate(content string) error {
	if len(content) == 0 || len(content) > 1024*1024 {
		return errors.New("Compose 内容为空或超过 1 MiB")
	}
	if interpolation.MatchString(strings.ReplaceAll(content, "$$", "")) {
		return errors.New("远程项目暂不支持环境变量插值，请填写明确值；容器内变量使用 $$ 转义")
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return errors.New("Compose YAML 格式无效")
	}
	if _, ok := doc["include"]; ok {
		return errors.New("远程项目暂不支持 include")
	}
	services, ok := doc["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return errors.New("Compose 必须包含 services")
	}
	for _, raw := range services {
		service, ok := raw.(map[string]any)
		if !ok {
			return errors.New("服务配置格式无效")
		}
		for _, key := range []string{"build", "env_file", "extends", "label_file", "develop"} {
			if _, ok := service[key]; ok {
				return errors.New("远程项目暂不支持 " + key + "，请使用独立镜像和内联配置")
			}
		}
	}
	for _, key := range []string{"configs", "secrets"} {
		if entries, ok := doc[key].(map[string]any); ok {
			for _, raw := range entries {
				if entry, ok := raw.(map[string]any); ok {
					if _, ok := entry["file"]; ok {
						return errors.New("远程项目暂不支持 " + key + " 的外部文件")
					}
					if _, ok := entry["environment"]; ok {
						return errors.New("远程项目暂不支持 " + key + " 的宿主环境输入")
					}
				}
			}
		}
	}
	return nil
}
