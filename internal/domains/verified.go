package domains

import (
	"context"
	"strings"

	"forkemail/internal/db"
)

func Verified(ctx context.Context, q db.Querier, workspaceID, fromAddress string) (bool, error) {
	domain := fromAddress[strings.LastIndex(fromAddress, "@")+1:]
	var verified bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM domains WHERE workspace_id = $1 AND status = 'verified'
			AND ((kind = 'domain' AND lower(name) = lower($2)) OR (kind = 'email' AND lower(name) = lower($3))))`,
		workspaceID, domain, fromAddress).Scan(&verified)
	return verified, err
}
