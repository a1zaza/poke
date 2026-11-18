package compose

import "testing"

func TestSelfContainedSource(t *testing.T) {
	for _, tc := range []struct {
		content string
		ok      bool
	}{
		{"services:\n  app:\n    image: registry.example/app:1\n    volumes: ['./data:/data']", true},
		{"services:\n  app:\n    image: app\n    command: 'echo $$HOME'", true},
		{"services:\n  app:\n    image: ${POKE_NODE_TOKEN}", false},
		{"services:\n  app:\n    image: app\n    env_file: .env", false},
		{"services:\n  app:\n    build: .", false},
		{"include: /host/private.yml\nservices:\n  app:\n    image: app", false},
		{"services:\n  app:\n    image: app\nsecrets:\n  token:\n    file: /host/token", false},
		{"services: []", false}, {"services: [", false},
	} {
		if err := Validate(tc.content); (err == nil) != tc.ok {
			t.Fatalf("valid=%v, err=%v, content=%s", tc.ok, err, tc.content)
		}
	}
}
