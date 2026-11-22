// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

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
