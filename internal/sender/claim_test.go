package sender

import (
	"context"
	"sync"
	"testing"

	"forkemail/internal/db/dbtest"
)

func TestClaimNeverReturnsSameRowTwice(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	email := dbtest.DraftEmail(t, pool, ws)
	dbtest.Exec(t, pool, `UPDATE emails SET status = 'sending', started_at = now() WHERE id = $1`, email)
	const total = 200
	dbtest.Exec(t, pool, `INSERT INTO email_recipients (email_id, workspace_id, email)
		SELECT $1, $2, 'r' || g || '@x.io' FROM generate_series(1, $3) g`, email, ws, total)

	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				batch, err := Claim(context.Background(), pool, ws, 7)
				if err != nil {
					t.Error(err)
					return
				}
				if len(batch) == 0 {
					return
				}
				mu.Lock()
				for _, r := range batch {
					seen[r.ID]++
					if r.Attempts != 1 {
						t.Errorf("attempts = %d, want 1", r.Attempts)
					}
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seen) != total {
		t.Fatalf("claimed %d distinct rows, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("row %s claimed %d times", id, n)
		}
	}
}
