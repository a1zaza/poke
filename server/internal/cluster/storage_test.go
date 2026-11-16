package cluster

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestEnrollmentPersistenceAndRevocation(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	n, token, err := s.CreateNode("测试节点", []string{"registry.example"})
	if err != nil {
		t.Fatal(err)
	}
	credential, attempt := secret(), randomID()
	if err = s.Enroll(n.ID, "wrong", attempt, credential); err == nil {
		t.Fatal("invalid invitation accepted")
	}
	if err = s.Enroll(n.ID, token, attempt, credential); err != nil {
		t.Fatal(err)
	}
	if err = s.Enroll(n.ID, token, attempt, credential); err != nil {
		t.Fatalf("lost enrollment response retry: %v", err)
	}
	if err = s.Enroll(n.ID, token, randomID(), secret()); err == nil {
		t.Fatal("invitation reused by another identity")
	}
	data, err := os.ReadFile(filepath.Join(dir, "nodes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(token)) || bytes.Contains(data, []byte(credential)) {
		t.Fatal("plaintext credential persisted in Controller")
	}
	s.Close()
	s, err = OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	auth, err := s.Authenticate(n.ID, credential)
	if err != nil || !auth.Registered {
		t.Fatalf("restart: %+v %v", auth, err)
	}
	if auth.CredentialHash != "" || auth.Attempt != "" {
		t.Fatal("private identity leaked")
	}
	auth.Registries[0] = "changed"
	auth, _ = s.Authenticate(n.ID, credential)
	if auth.Registries[0] != "registry.example" {
		t.Fatal("mutable registry snapshot")
	}
	if err = s.Revoke(n.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(n.ID, credential); err == nil {
		t.Fatal("revoked identity accepted")
	}
	next, err := s.RenewEnrollment(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Enroll(n.ID, token, attempt, credential); err == nil {
		t.Fatal("old identity resurrected")
	}
	if err = s.Enroll(n.ID, next, randomID(), secret()); err != nil {
		t.Fatal(err)
	}
}
func TestJSONCorruptionLockAndRecovery(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = OpenStore(dir); err == nil {
		t.Fatal("two writers accepted")
	}
	task := Task{ID: randomID(), NodeID: randomID(), State: "running", CreatedAt: time.Now()}
	if err = s.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverTasks(); err != nil {
		t.Fatal(err)
	}
	after, err := s.Task(task.ID)
	if err != nil || after.State != "unknown" {
		t.Fatalf("recovery: %+v %v", after, err)
	}
	info, err := os.Stat(filepath.Join(dir, "tasks", task.ID+".json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v %v", info, err)
	}
	s.Close()
	bad := []byte("{broken-json")
	if err = os.WriteFile(filepath.Join(dir, "nodes.json"), bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenStore(dir); err == nil {
		t.Fatal("corrupt store accepted")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "nodes.json"))
	if !bytes.Equal(data, bad) {
		t.Fatal("corrupt state overwritten")
	}
}
func TestConcurrentNodeCreationAndAtomicFailure(t *testing.T) {
	s := openTestStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := s.CreateNode("节点", nil); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(s.Nodes()) != 12 {
		t.Fatal("concurrent update lost")
	}
	var disk nodeFile
	if err := readJSON(filepath.Join(s.dir, "nodes.json"), &disk); err != nil || len(disk.Nodes) != 12 {
		t.Fatalf("disk: %v", err)
	}
	original := s.dir
	s.dir = filepath.Join(s.dir, "missing")
	if _, _, err := s.CreateNode("不能写入", nil); err == nil {
		t.Fatal("write failure ignored")
	}
	s.dir = original
	if len(s.Nodes()) != 12 {
		t.Fatal("failed write published in memory")
	}
	public, _ := json.Marshal(s.Nodes())
	if bytes.Contains(public, []byte("enrollment_hash")) {
		t.Fatal("hash leaked through public view")
	}
}
func TestExpiredEnrollmentAndInvalidTasks(t *testing.T) {
	s := openTestStore(t)
	n, token, err := s.CreateNode("过期", nil)
	if err != nil {
		t.Fatal(err)
	}
	stored := s.nodes[n.ID]
	stored.EnrollmentExpires = time.Now().Add(-time.Second)
	s.nodes[n.ID] = stored
	if err = s.Enroll(n.ID, token, randomID(), secret()); err == nil {
		t.Fatal("expired invitation accepted")
	}
	if err = s.SaveTask(Task{ID: "../../outside"}); err == nil {
		t.Fatal("unsafe task path")
	}
}
