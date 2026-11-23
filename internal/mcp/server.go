// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package mcp

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed docs.md
var docsSource string

var Guide = strings.Replace(docsSource, "{{tools}}", toolsMarkdown(), 1)

const serverVersion = "1.0.0"

var protocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

type Handler struct {
	Pool  *pgxpool.Pool
	Auth  *auth.Authenticator
	API   http.Handler
	Limit func(http.Handler) http.Handler

	Mail          Mailer
	MailWorkspace string
	MailFrom      string
	BaseURL       string
	Secret        []byte
}

type rpcRequest struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Mcp-Protocol-Version, Mcp-Session-Id")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate")
	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
		return
	case http.MethodPost:
	default:
		w.Header().Set("Allow", "POST, OPTIONS")
		httpx.Error(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	header := r.Header.Get("Authorization")
	id, err := h.Auth.IdentifyBearer(r.Context(), header)
	if errors.Is(err, auth.ErrUnauthorized) {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+h.BaseURL+`/.well-known/oauth-protected-resource"`)
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.SetLogFields(r.Context(), id.WorkspaceID, id.KeyPrefix)
	token, _ := strings.CutPrefix(header, "Bearer ")
	serve := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.serve(w, r, id, token) })
	if h.Limit != nil {
		h.Limit(serve).ServeHTTP(w, r.WithContext(auth.WithIdentity(r.Context(), id)))
		return
	}
	serve(w, r)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request, id auth.Identity, token string) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "body_too_large")
		return
	}
	sess := &session{h: h, id: id, token: token, ctx: r.Context()}
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '[' {
		var batch []rpcRequest
		if json.Unmarshal(raw, &batch) != nil {
			writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			return
		}
		var out []rpcResponse
		for _, req := range batch {
			if res, ok := sess.handle(req); ok {
				out = append(out, res)
			}
		}
		if len(out) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeRPC(w, out)
		return
	}
	var req rpcRequest
	if json.Unmarshal(raw, &req) != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
		return
	}
	res, ok := sess.handle(req)
	if !ok {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeRPC(w, res)
}

func writeRPC(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

type session struct {
	h     *Handler
	id    auth.Identity
	token string
	ctx   context.Context
}

func (s *session) handle(req rpcRequest) (rpcResponse, bool) {
	if len(req.ID) == 0 {
		return rpcResponse{}, false
	}
	res := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := protocolVersions[0]
		for _, v := range protocolVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		res.Result = map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}, "resources": map[string]any{}},
			"serverInfo":      map[string]any{"name": "forke-mail", "title": "Forke Mail", "version": serverVersion},
			"instructions":    instructions,
		}
	case "ping":
		res.Result = map[string]any{}
	case "tools/list":
		res.Result = map[string]any{"tools": toolList()}
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			res.Error = &rpcError{-32602, "invalid params"}
			break
		}
		t, ok := toolByName[p.Name]
		if !ok {
			res.Error = &rpcError{-32602, "unknown tool: " + p.Name}
			break
		}
		if p.Arguments == nil {
			p.Arguments = map[string]any{}
		}
		res.Result = s.callTool(t, p.Arguments)
	case "resources/list":
		res.Result = map[string]any{"resources": []map[string]any{{"uri": "forke-mail://guide", "name": "guide", "title": "Forke Mail agent guide", "mimeType": "text/markdown"}}}
	case "resources/read":
		res.Result = map[string]any{"contents": []map[string]any{{"uri": "forke-mail://guide", "mimeType": "text/markdown", "text": Guide}}}
	case "resources/templates/list":
		res.Result = map[string]any{"resourceTemplates": []any{}}
	case "prompts/list":
		res.Result = map[string]any{"prompts": []any{}}
	default:
		res.Error = &rpcError{-32601, "method not found: " + req.Method}
	}
	return res, true
}

const instructions = `Forke Mail sends transactional emails and broadcasts through Amazon SES. Start with get_account to see the project, credits, verified domains and whether you are using a test key.
Safety: some tools answer with CONFIRMATION NEEDED or APPROVAL NEEDED instead of acting. For CONFIRMATION NEEDED, show the user the summary, wait for an explicit yes in the chat, then call the same tool again with the same arguments plus confirm_token. Never confirm on the user's behalf. For APPROVAL NEEDED, give the user the link; they approve in their browser, then you call again with approval_id.
Money: never try to buy, pay or add credits yourself. If a send fails for credits or limits, tell the user what to do.
Building an app: create a test API key (create_api_key with mode "test") for development; test sends are stored but never delivered. Read the resource forke-mail://guide for the HTTP API and code samples.`

type gate struct {
	risk    Risk
	summary string
}

func (g *gate) Error() string { return "needs " + riskName[g.risk] }

type apiError struct {
	status int
	code   string
	body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("%d %s", e.status, e.code) }
