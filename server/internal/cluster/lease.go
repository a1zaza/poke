package cluster

import (
	"errors"
	"time"
)

var ErrNodeConnected = errors.New("NODE_ALREADY_CONNECTED")
var ErrReconnectPending = errors.New("NODE_RECONNECT_PENDING")

// Caller serializes this with its in-memory session replacement. Authentication
// is repeated under the same store lock as lease ownership and credential version.
func (s *Store) Claim(id, token, boot string) (Node, error) {
	if !safeID.MatchString(boot) {
		return Node{}, ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok || n.Revoked || !matches(token, n.CredentialHash) {
		return Node{}, ErrUnauthorized
	}
	if n.BootID != "" && n.BootID != boot && time.Now().Before(n.LeaseUntil) {
		return Node{}, ErrNodeConnected
	}
	n.BootID = boot
	n.Epoch++
	n.LeaseUntil = time.Now().Add(LeaseDuration).UTC()
	m := s.copyNodes()
	m[id] = n
	if err := s.saveNodes(m); err != nil {
		return Node{}, err
	}
	return n.Public(), nil
}
func (s *Store) RenewLease(id, boot string, epoch, credentialVersion uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok || n.Revoked || n.BootID != boot || n.Epoch != epoch || n.CredentialVersion != credentialVersion || !time.Now().Before(n.LeaseUntil) {
		return ErrUnauthorized
	}
	n.LeaseUntil = time.Now().Add(LeaseDuration).UTC()
	m := s.copyNodes()
	m[id] = n
	return s.saveNodes(m)
}
func (s *Store) ReleaseLease(id, boot string, epoch uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok || n.BootID != boot || n.Epoch != epoch {
		return ErrUnauthorized
	}
	n.LeaseUntil = time.Time{}
	m := s.copyNodes()
	m[id] = n
	return s.saveNodes(m)
}
func (s *Store) ValidLease(id, boot string, epoch uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[id]
	return ok && !n.Revoked && n.BootID == boot && n.Epoch == epoch && time.Now().Before(n.LeaseUntil)
}
func (s *Store) NodeBlocked(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.tasks {
		if t.NodeID == id && t.State == "unknown" && !t.Resolved {
			return true
		}
	}
	return false
}

// A new boot cannot recover the previous process's volatile task journal.
func (s *Store) RecoverNode(id, boot string) error {
	for _, meta := range s.allTasks() {
		if meta.NodeID != id || meta.Terminal() || meta.BootID == boot {
			continue
		}
		if err := s.UpdateTask(meta.ID, func(t *Task) {
			if t.Terminal() {
				return
			}
			if t.Permit == "" {
				t.State = "cancelled"
				t.Error = "节点进程已更换，未执行任务已取消"
			} else {
				t.State = "unknown"
				t.Error = "节点进程已更换，执行结果待确认；不会自动重放"
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

// A successful socket write is not proof the Agent received a task. Reconcile
// its volatile journal on every handshake, including reconnects of the same boot.
func (s *Store) ReconcilePending(node, boot string, known []string) error {
	present := map[string]bool{}
	for _, id := range known {
		if !safeID.MatchString(id) {
			return errors.New("待确认任务标识无效")
		}
		present[id] = true
	}
	for _, meta := range s.allTasks() {
		if meta.NodeID != node || meta.BootID != boot || meta.Terminal() || present[meta.ID] {
			continue
		}
		if err := s.UpdateTask(meta.ID, func(t *Task) {
			if t.Terminal() {
				return
			}
			if t.Permit == "" {
				t.State = "cancelled"
				t.Error = "节点重连未确认接收该任务，未授权执行，已取消"
			} else {
				t.State = "unknown"
				t.Error = "节点重连丢失已许可任务的状态，请核实执行结果"
			}
		}); err != nil {
			return err
		}
	}
	return nil
}
