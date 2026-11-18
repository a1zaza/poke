package cluster

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLeaseAtomicDuplicateRestartAndRotation(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	n, token, err := s.CreateNode("节点", nil)
	if err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int32
	var owner atomic.Value
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			boot := randomID()
			lease, err := s.Claim(n.ID, token, boot)
			if err == nil {
				winners.Add(1)
				owner.Store(lease)
			} else if err != ErrNodeConnected {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatal("duplicate node instances admitted", winners.Load())
	}
	lease := owner.Load().(Node)
	next, err := s.Claim(n.ID, token, lease.BootID)
	if err != nil || next.Epoch <= lease.Epoch {
		t.Fatal("same boot reconnect failed", err)
	}
	if err := s.ReleaseLease(n.ID, lease.BootID, lease.Epoch); err == nil {
		t.Fatal("old session released new lease")
	}
	s.Close()
	s, err = OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(n.ID, token, randomID()); err != ErrNodeConnected {
		t.Fatal("controller restart lost lease", err)
	}
	if !s.ValidLease(n.ID, next.BootID, next.Epoch) {
		t.Fatal("lease not restored")
	}
	rotated, err := s.RotateToken(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(n.ID, token); err == nil {
		t.Fatal("old token survived rotation")
	}
	if _, err := s.Claim(n.ID, rotated, randomID()); err != ErrNodeConnected {
		t.Fatal("rotation bypassed live instance", err)
	}
	if err := s.ReleaseLease(n.ID, next.BootID, next.Epoch); err == nil {
		t.Fatal("rotated token's old session released lease")
	}
	current := s.Nodes()[0]
	if err := s.ReleaseLease(n.ID, current.BootID, current.Epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(n.ID, rotated, randomID()); err != nil {
		t.Fatal("released lease not reusable", err)
	}
	if len(s.Nodes()) != 1 {
		t.Fatal("reconnect added duplicate nodes")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "nodes.json"))
	if bytes.Contains(raw, []byte(token)) {
		t.Fatal("token leaked")
	}
}
func TestRecoverNewBootAndUnknownBlock(t *testing.T) {
	s := openTestStore(t)
	node, oldBoot, newBoot := randomID(), randomID(), randomID()
	queue := Task{ID: randomID(), NodeID: node, BootID: oldBoot, State: "queued", Request: &Request{Method: "POST", Path: "/api/containers/demo/start"}}
	granted := queue
	granted.ID = randomID()
	granted.Permit = randomID()
	granted.State = "execution_granted"
	for _, task := range []Task{queue, granted} {
		if err := s.SaveTask(task); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecoverNode(node, newBoot); err != nil {
		t.Fatal(err)
	}
	q, _ := s.Task(queue.ID)
	g, _ := s.Task(granted.ID)
	if q.State != "cancelled" || g.State != "unknown" || g.Request == nil || !s.NodeBlocked(node) {
		t.Fatal("unsafe recovery", q, g)
	}
	if err := s.UpdateTask(g.ID, func(t *Task) { t.Resolved = true }); err != nil {
		t.Fatal(err)
	}
	if s.NodeBlocked(node) {
		t.Fatal("manual resolution did not unblock")
	}
}
func TestLeaseExpiryAndWriteFailure(t *testing.T) {
	s := openTestStore(t)
	n, token, _ := s.CreateNode("node", nil)
	lease, err := s.Claim(n.ID, token, randomID())
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	expired := s.nodes[n.ID]
	expired.LeaseUntil = time.Now().Add(-time.Second)
	m := s.copyNodes()
	m[n.ID] = expired
	if err := s.saveNodes(m); err != nil {
		t.Fatal(err)
	}
	s.mu.Unlock()
	if err := s.RenewLease(n.ID, lease.BootID, lease.Epoch, n.CredentialVersion); err == nil {
		t.Fatal("expired lease renewed")
	}
	if _, err := s.Claim(n.ID, token, randomID()); err != nil {
		t.Fatal("expired lease blocked takeover", err)
	}
	current := s.Nodes()[0]
	original := s.dir
	s.dir = filepath.Join(original, "missing")
	if _, err := s.Claim(n.ID, token, current.BootID); err == nil {
		t.Fatal("failed persistence published lease")
	}
	s.dir = original
	if s.Nodes()[0].Epoch != current.Epoch {
		t.Fatal("failed lease write changed in-memory epoch")
	}
}

func TestReconnectReconcilesLostDelivery(t *testing.T) {
	s := openTestStore(t)
	node, boot := randomID(), randomID()
	missing := Task{ID: randomID(), NodeID: node, BootID: boot, State: "queued"}
	uncertain := missing
	uncertain.ID = randomID()
	uncertain.State = "execution_granted"
	uncertain.Permit = randomID()
	known := missing
	known.ID = randomID()
	for _, task := range []Task{missing, uncertain, known} {
		if err := s.SaveTask(task); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ReconcilePending(node, boot, []string{known.ID}); err != nil {
		t.Fatal(err)
	}
	m, _ := s.Task(missing.ID)
	u, _ := s.Task(uncertain.ID)
	k, _ := s.Task(known.ID)
	if m.State != "cancelled" || u.State != "unknown" || k.State != "queued" {
		t.Fatal("reconnect replayed or stranded lost task", m, u, k)
	}
}
